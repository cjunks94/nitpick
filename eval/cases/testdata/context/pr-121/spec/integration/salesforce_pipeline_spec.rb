require "rails_helper"

# rubocop:disable RSpec/DescribeClass — integration spec, not a single-class unit test
RSpec.describe "Salesforce pipeline integration" do
  let(:org) { create(:org) }
  let(:mock_client) { instance_double(Restforce::Client) }
  let(:sf_connection) do
    create(:connection, org: org, db_type: "salesforce", connection_config: {
      "instance_url" => "https://test.salesforce.com",
      "sandbox" => true,
      "credentials" => {
        "client_id" => "test_id",
        "client_secret" => "test_secret",
        "username" => "test@example.com",
        "password" => "pass",
        "security_token" => "token"
      }
    })
  end

  before do
    Current.org = org
    allow(Restforce).to receive(:new).and_return(mock_client)
    allow(mock_client).to receive(:authenticate!).and_return(true)
  end

  after { Current.org = nil }

  describe "Registry resolution" do
    it "resolves SalesforceAdapter for salesforce db_type" do
      adapter = Sources::Registry.adapter_for(sf_connection)
      expect(adapter).to be_a(Sources::SalesforceAdapter)
    end
  end

  describe "ProbeJob" do
    it "updates connection status to ok on successful probe" do
      allow(mock_client).to receive(:describe).with("Account").and_return({ "name" => "Account" })

      Connections::ProbeJob.perform_now(sf_connection.id)
      sf_connection.reload

      expect(sf_connection.connection_status).to eq("ok")
      expect(sf_connection.connection_error).to be_nil
    end

    it "updates connection status to error on failed probe" do
      allow(mock_client).to receive(:describe).with("Account")
        .and_raise(Restforce::AuthenticationError.new("invalid_grant"))

      Connections::ProbeJob.perform_now(sf_connection.id)
      sf_connection.reload

      expect(sf_connection.connection_status).to eq("error")
      expect(sf_connection.connection_error).to include("invalid_grant")
    end
  end

  describe "SchemaIntrospector" do
    let(:object_list) { [ { "name" => "Account", "queryable" => true } ] }
    let(:account_describe) do
      { "fields" => [
        { "name" => "Id", "type" => "id", "nillable" => false },
        { "name" => "Name", "type" => "string", "nillable" => false }
      ] }
    end

    it "returns Salesforce objects via the adapter" do
      allow(mock_client).to receive(:describe).with(no_args).and_return(object_list)
      allow(mock_client).to receive(:describe).with("Account").and_return(account_describe)

      schema = Connections::SchemaIntrospector.call(sf_connection, force: true)

      expect(schema.first[:table]).to eq("Account")
      expect(schema.first[:columns].first[:name]).to eq("Id")
    end
  end

  describe "Exports::Executor with Salesforce source" do
    let(:mapping) do
      create(:mapping, org: org, fields: [
        { "source" => "Id", "target" => "sf_id" },
        { "source" => "Name", "target" => "account_name" }
      ])
    end
    let(:pipeline) do
      create(:pipeline, org: org, connection: sf_connection, mapping: mapping,
             query_text: "SELECT Id, Name FROM Account", transforms: [])
    end
    let(:destination) { create(:destination, org: org, config: {}) }
    let(:export) { create(:export, org: org, pipeline: pipeline, destination: destination, name: "sf-test") }
    let(:pipeline_run) { create(:pipeline_run, org: org, pipeline: pipeline) }
    let(:compiled) { create(:compiled_definition, org: org) }
    let(:sf_records) do
      [
        Restforce::SObject.new("Id" => "001A", "Name" => "Acme Corp", "attributes" => { "type" => "Account" }),
        Restforce::SObject.new("Id" => "001B", "Name" => "Globex Inc", "attributes" => { "type" => "Account" })
      ]
    end

    it "creates a successful ExportRun with correct row count", :aggregate_failures do
      allow(mock_client).to receive(:query).with("SELECT Id, Name FROM Account").and_return(sf_records)

      Exports::Executor.call(export, pipeline_run: pipeline_run, compiled: compiled)
      export_run = ExportRun.last

      expect(export_run.status).to eq("success")
      expect(export_run.row_count).to eq(2)
      expect(export_run.artifact).to be_attached
    end

    it "records performance metrics", :aggregate_failures do
      allow(mock_client).to receive(:query).with("SELECT Id, Name FROM Account").and_return(sf_records)

      Exports::Executor.call(export, pipeline_run: pipeline_run, compiled: compiled)
      export_run = ExportRun.last

      expect(export_run.metrics).to include("extraction_ms" => be_a(Integer))
      expect(export_run.metrics).to include("rows_per_second" => be_a(Float))
    end

    it "writes CSV with mapped column names" do
      allow(mock_client).to receive(:query).with("SELECT Id, Name FROM Account").and_return(sf_records)

      Exports::Executor.call(export, pipeline_run: pipeline_run, compiled: compiled)
      csv_content = ExportRun.last.artifact.download

      expect(csv_content).to include("sf_id,account_name")
      expect(csv_content).to include("001A,Acme Corp")
    end

    it "records failure when Salesforce query errors" do
      allow(mock_client).to receive(:query)
        .and_raise(Restforce::NotFoundError.new("INVALID_TYPE"))

      expect {
        Exports::Executor.call(export, pipeline_run: pipeline_run, compiled: compiled)
      }.to raise_error(Sources::AdapterError)

      export_run = ExportRun.last
      expect(export_run.status).to eq("failed")
      expect(export_run.error_class).to eq("Sources::AdapterError")
    end
  end
end
# rubocop:enable RSpec/DescribeClass
