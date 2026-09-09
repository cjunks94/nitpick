module Api
  module V1
    class BaseController < ActionController::API
      include ActionController::HttpAuthentication::Basic::ControllerMethods
      include Pundit::Authorization
      include Pagy::Method

      before_action :authenticate!
      before_action :set_org

      rescue_from ActiveRecord::RecordNotFound, with: :not_found
      rescue_from ActiveRecord::RecordInvalid, with: :unprocessable
      rescue_from ArgumentError, with: :bad_request_with_message
      rescue_from Discard::RecordNotDiscarded, with: :unprocessable_discard
      rescue_from Pundit::NotAuthorizedError, with: :forbidden

      private

      def authenticate!
        if auth_disabled?
          return
        elsif !auth_configured?
          render_error("API authentication is not configured. Set EXPORTEE_AUTH_USER and " \
                       "EXPORTEE_AUTH_PASSWORD, or set EXPORTEE_AUTH_DISABLED=true to allow " \
                       "unauthenticated access.", :forbidden, code: "auth_not_configured")
          return
        end

        # Bitwise & (not &&) is intentional: && short-circuits on username
        # failure, skipping the password check and leaking timing info about
        # which credential was wrong. & always evaluates both sides.
        authenticate_or_request_with_http_basic("Exportee API") do |username, password|
          ActiveSupport::SecurityUtils.secure_compare(username, ENV["EXPORTEE_AUTH_USER"]) &
            ActiveSupport::SecurityUtils.secure_compare(password, ENV["EXPORTEE_AUTH_PASSWORD"])
        end
      end

      def auth_configured?
        ENV["EXPORTEE_AUTH_USER"].present? && ENV["EXPORTEE_AUTH_PASSWORD"].present?
      end

      def auth_disabled?
        ENV["EXPORTEE_AUTH_DISABLED"].present?
      end

      def set_org
        @org = Org.first
        return render_error("No organization configured", :not_found) unless @org
        Current.org = @org
      end

      def render_error(message, status, code: nil)
        render json: { error: { message: message, code: code || status.to_s } }, status: status
      end

      def not_found
        render_error("Resource not found", :not_found, code: "not_found")
      end

      def unprocessable(exception)
        render_error(exception.record.errors.full_messages.join(", "),
                     :unprocessable_entity, code: "validation_failed")
      end

      def forbidden(exception)
        render_error("Not authorized to #{exception.query.to_s.chomp('?')} this resource",
                     :forbidden, code: "forbidden")
      end

      # Phase 1: no real user sessions — API uses shared HTTP Basic creds.
      # Pundit needs a user object. Fall back to the org's owner so policies
      # are exercised. Replace with real user resolution when auth lands.
      def pundit_user
        Current.user || @org&.memberships&.find_by(role: "owner")&.user
      end

      def bad_request
        render_error("Invalid request parameters", :bad_request, code: "bad_request")
      end

      def bad_request_with_message(exception)
        render_error(exception.message, :bad_request, code: "bad_request")
      end

      def unprocessable_discard(exception)
        render_error(exception.message, :unprocessable_entity, code: "discard_failed")
      end

      def pagy_meta(pagy_obj)
        {
          current_page: pagy_obj.page,
          total_pages: pagy_obj.pages,
          total_count: pagy_obj.count,
          per_page: pagy_obj.limit
        }
      end
    end
  end
end
