module Api
  module V1
    class WidgetsController < BaseController
      def index
        authorize Widget
        widgets = Widget.for_org(@org).kept.order(:name)
        pagy_obj, records = pagy(:offset, widgets)

        render json: {
          data: records.map { |w| serialize(w) },
          meta: pagy_meta(pagy_obj)
        }
      end

      def show
        widget = find_widget
        authorize widget
        render json: { data: serialize(widget) }
      end

      def create
        widget = Widget.new(create_params)
        widget.org = @org
        authorize widget
        widget.save!
        render json: { data: serialize(widget) }, status: :created
      end

      def update
        widget = find_widget
        authorize widget
        widget.update!(update_params)
        render json: { data: serialize(widget) }
      end

      def destroy
        widget = find_widget
        authorize widget
        widget.discard!
        head :no_content
      end

      private

      def find_widget
        Widget.for_org(@org).kept.find_by!(uuid: params[:id])
      end

      def create_params
        params.require(:widget).permit(:name, :description, :widget_type, :is_public, config: {}, tags: [])
      end

      def update_params
        params.require(:widget).permit(:name, :description, :is_public, config: {}, tags: [])
      end

      def serialize(widget)
        {
          uuid: widget.uuid,
          name: widget.name,
          description: widget.description,
          widget_type: widget.widget_type,
          config: widget.config,
          is_public: widget.is_public,
          tags: widget.tags,
          version: widget.version,
          created_at: widget.created_at.iso8601,
          updated_at: widget.updated_at.iso8601
        }
      end
    end
  end
end
