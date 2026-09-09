require "swagger_helper"

# rubocop:disable RSpec/EmptyExampleGroup, RSpec/VariableName, RSpec/ScatteredSetup
RSpec.describe "API V1 Widgets", type: :request do
  let!(:org) { create(:org) }
  let!(:owner_user) { create(:user, email: "owner@test.com") }
  let(:Authorization) { "Basic #{Base64.strict_encode64('admin:s3cret')}" }

  # Ensure owner membership exists so pundit_user fallback works
  before { create(:membership, user: owner_user, org: org, role: "owner") }

  around do |example|
    ClimateControl.modify("EXPORTEE_AUTH_USER" => "admin", "EXPORTEE_AUTH_PASSWORD" => "s3cret") do
      example.run
    end
  end

  path "/api/v1/widgets" do
    get "List widgets" do
      tags "Widgets"
      produces "application/json"
      parameter name: :page, in: :query, type: :integer, required: false
      parameter name: :limit, in: :query, type: :integer, required: false

      response "200", "widgets listed" do
        schema type: :object, properties: {
          data: { type: :array, items: { "$ref": "#/components/schemas/widget" } },
          meta: { "$ref": "#/components/schemas/pagination_meta" }
        }, required: %w[data meta]

        before { create_list(:widget, 2, org: org) }

        run_test! do |response|
          data = JSON.parse(response.body)
          expect(data["data"].length).to eq(2)
          expect(data["meta"]).to include("current_page", "total_count")
          expect(data["data"]).to all(include("uuid", "name", "widget_type", "version"))
        end
      end
    end

    post "Create a widget" do
      tags "Widgets"
      consumes "application/json"
      produces "application/json"
      parameter name: :widget, in: :body, schema: {
        type: :object,
        properties: {
          widget: {
            type: :object,
            properties: {
              name: { type: :string },
              widget_type: { type: :string, enum: %w[rename filter mask_ssn mask_email] },
              description: { type: :string },
              config: { type: :object },
              tags: { type: :array, items: { type: :string } },
              is_public: { type: :boolean }
            },
            required: %w[name widget_type]
          }
        }
      }

      response "201", "widget created" do
        schema type: :object, properties: {
          data: { "$ref": "#/components/schemas/widget" }
        }, required: %w[data]

        let(:widget) do
          {
            widget: {
              name: "mask-pii-email",
              widget_type: "mask_email",
              description: "Masks email local part",
              config: { "on" => "email" },
              tags: %w[pii compliance],
              is_public: false
            }
          }
        end

        run_test! do |response|
          data = JSON.parse(response.body)["data"]
          expect(data["name"]).to eq("mask-pii-email")
          expect(data["widget_type"]).to eq("mask_email")
          expect(data["tags"]).to eq(%w[pii compliance])
          expect(data["config"]).to eq({ "on" => "email" })
        end
      end

      response "422", "invalid params" do
        schema "$ref": "#/components/schemas/error"

        let(:widget) { { widget: { name: "", widget_type: "mask_email" } } }
        run_test!
      end
    end
  end

  path "/api/v1/widgets/{id}" do
    get "Show a widget" do
      tags "Widgets"
      produces "application/json"
      parameter name: :id, in: :path, type: :string, description: "Widget UUID"

      response "200", "widget found" do
        schema type: :object, properties: {
          data: { "$ref": "#/components/schemas/widget" }
        }, required: %w[data]

        let(:existing_widget) { create(:widget, org: org) }
        let(:id) { existing_widget.uuid }

        run_test! do |response|
          data = JSON.parse(response.body)["data"]
          expect(data["uuid"]).to eq(existing_widget.uuid)
          expect(data["widget_type"]).to eq(existing_widget.widget_type)
        end
      end

      response "404", "widget not found" do
        schema "$ref": "#/components/schemas/error"
        let(:id) { "nonexistent-uuid" }
        run_test!
      end
    end

    patch "Update a widget" do
      tags "Widgets"
      consumes "application/json"
      produces "application/json"
      parameter name: :id, in: :path, type: :string, description: "Widget UUID"
      parameter name: :widget, in: :body, schema: {
        type: :object,
        properties: {
          widget: {
            type: :object,
            properties: {
              name: { type: :string },
              description: { type: :string },
              config: { type: :object },
              tags: { type: :array, items: { type: :string } },
              is_public: { type: :boolean }
            }
          }
        }
      }

      response "200", "widget updated" do
        schema type: :object, properties: {
          data: { "$ref": "#/components/schemas/widget" }
        }, required: %w[data]

        let(:existing_widget) { create(:widget, org: org, tags: []) }
        let(:id) { existing_widget.uuid }
        let(:widget) { { widget: { tags: %w[pii sensitive], description: "Updated" } } }

        run_test! do |response|
          data = JSON.parse(response.body)["data"]
          expect(data["tags"]).to eq(%w[pii sensitive])
          expect(data["description"]).to eq("Updated")
        end
      end

      response "404", "widget not found" do
        schema "$ref": "#/components/schemas/error"
        let(:id) { "nonexistent-uuid" }
        let(:widget) { { widget: { description: "nope" } } }
        run_test!
      end
    end

    delete "Delete a widget" do
      tags "Widgets"
      produces "application/json"
      parameter name: :id, in: :path, type: :string, description: "Widget UUID"

      response "204", "widget deleted" do
        let(:existing_widget) { create(:widget, org: org) }
        let(:id) { existing_widget.uuid }

        run_test! do
          expect(existing_widget.reload).to be_discarded
        end
      end

      response "404", "widget not found" do
        let(:id) { "nonexistent-uuid" }
        run_test!
      end
    end
  end
end
# rubocop:enable RSpec/EmptyExampleGroup, RSpec/VariableName, RSpec/ScatteredSetup
