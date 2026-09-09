require "rails_helper"

RSpec.describe Sources::SalesforceAdapter do
  subject(:adapter) { described_class.new(connection) }

  let(:connection) do
    build_stubbed(:connection, db_type: "salesforce", connection_config: {
      "instance_url" => "https://test.salesforce.com",
      "api_version" => "61.0",
      "sandbox" => true,
      "credentials" => {
        "client_id" => "test_client_id",
        "client_secret" => "test_client_secret",
        "username" => "test@example.com",
        "password" => "testpass",
        "security_token" => "testtoken"
      }
    })
  end

  let(:mock_client) { instance_double(Restforce::Client) }

  before do
    allow(Restforce).to receive(:new).and_return(mock_client)
    allow(mock_client).to receive(:authenticate!).and_return(true)
  end

  it_behaves_like "a source adapter" do
    before do
      allow(mock_client).to receive(:describe).with("Account").and_return({})
      allow(mock_client).to receive(:describe).with(no_args).and_return([])
      allow(mock_client).to receive(:query).and_return([])
    end
  end

  describe "#probe" do
    it "returns ok when Salesforce is reachable" do
      allow(mock_client).to receive(:describe).with("Account").and_return({ "name" => "Account" })

      result = adapter.probe
      expect(result[:ok]).to be(true)
      expect(result[:error]).to be_nil
    end

    it "returns error when authentication fails" do
      allow(mock_client).to receive(:describe).with("Account")
        .and_raise(Restforce::AuthenticationError.new("invalid_grant"))

      result = adapter.probe
      expect(result[:ok]).to be(false)
      expect(result[:error]).to include("invalid_grant")
    end
  end

  describe "#introspect_schema" do
    let(:object_list) do
      [
        { "name" => "Account", "queryable" => true },
        { "name" => "ApexLog", "queryable" => false }
      ]
    end
    let(:account_describe) do
      { "fields" => [
        { "name" => "Id", "type" => "id", "nillable" => false },
        { "name" => "Name", "type" => "string", "nillable" => false },
        { "name" => "Industry", "type" => "picklist", "nillable" => true }
      ] }
    end
    let(:expected_columns) do
      [
        { name: "Id", type: "id", nullable: false },
        { name: "Name", type: "string", nullable: false },
        { name: "Industry", type: "picklist", nullable: true }
      ]
    end

    it "returns objects and their fields" do
      allow(mock_client).to receive(:describe).with(no_args).and_return(object_list)
      allow(mock_client).to receive(:describe).with("Account").and_return(account_describe)

      schema = adapter.introspect_schema

      expect(schema.length).to eq(1)
      expect(schema.first[:table]).to eq("Account")
      expect(schema.first[:columns]).to match_array(expected_columns)
    end

    it "skips non-queryable objects" do
      allow(mock_client).to receive(:describe).with(no_args).and_return([
        { "name" => "ApexLog", "queryable" => false }
      ])

      schema = adapter.introspect_schema
      expect(schema).to be_empty
    end
  end

  describe "#extract" do
    it "returns normalized row hashes from SOQL query" do
      records = [
        Restforce::SObject.new("Id" => "001A", "Name" => "Acme", "attributes" => { "type" => "Account" }),
        Restforce::SObject.new("Id" => "001B", "Name" => "Globex", "attributes" => { "type" => "Account" })
      ]
      allow(mock_client).to receive(:query).with("SELECT Id, Name FROM Account").and_return(records)

      rows = adapter.extract("SELECT Id, Name FROM Account")

      expect(rows.length).to eq(2)
      expect(rows.first).to eq({ "Id" => "001A", "Name" => "Acme" })
      expect(rows.first).not_to have_key("attributes")
    end

    it "returns empty array for no results" do
      allow(mock_client).to receive(:query).and_return([])

      rows = adapter.extract("SELECT Id FROM Account WHERE Id = 'nonexistent'")
      expect(rows).to eq([])
    end

    it "raises AdapterError on Salesforce error" do
      allow(mock_client).to receive(:query).and_raise(Restforce::NotFoundError.new("not found"))

      expect {
        adapter.extract("SELECT Id FROM BadObject__c")
      }.to raise_error(Sources::AdapterError, /Salesforce query failed/)
    end
  end

  describe "#extract_streaming" do
    it "delegates to extract" do
      allow(mock_client).to receive(:query).and_return([])

      expect(adapter.extract_streaming("SELECT Id FROM Account")).to eq([])
    end
  end

  describe "error handling" do
    it "handles network timeouts gracefully" do
      allow(mock_client).to receive(:describe).with("Account")
        .and_raise(Faraday::TimeoutError.new("request timed out"))

      result = adapter.probe
      expect(result[:ok]).to be(false)
      expect(result[:error]).to include("timed out")
    end

    it "handles connection refused" do
      allow(mock_client).to receive(:describe).with("Account")
        .and_raise(Faraday::ConnectionFailed.new("connection refused"))

      result = adapter.probe
      expect(result[:ok]).to be(false)
      expect(result[:error]).to include("connection refused")
    end

    it "wraps introspection errors in AdapterError" do
      allow(mock_client).to receive(:describe).with(no_args)
        .and_raise(Restforce::UnauthorizedError.new("Session expired"))

      expect {
        adapter.introspect_schema
      }.to raise_error(Sources::AdapterError, /introspection failed/)
    end
  end

  describe "credentials handling" do
    let(:no_token_config) do
      {
        "instance_url" => "https://test.salesforce.com",
        "sandbox" => true,
        "credentials" => {
          "client_id" => "id", "client_secret" => "secret",
          "username" => "user@test.com", "password" => "pass"
        }
      }
    end

    it "works without security_token" do
      no_token_connection = build_stubbed(:connection, db_type: "salesforce", connection_config: no_token_config)
      no_token_adapter = described_class.new(no_token_connection)
      allow(mock_client).to receive(:describe).with("Account").and_return({})

      no_token_adapter.probe

      expect(Restforce).to have_received(:new).with(hash_including(password: "pass"))
    end

    it "reads credentials from nested credentials key" do
      allow(mock_client).to receive(:describe).with("Account").and_return({})
      adapter.probe

      expect(Restforce).to have_received(:new).with(hash_including(
        username: "test@example.com",
        client_id: "test_client_id"
      ))
    end
  end

  describe "authentication" do
    let(:prod_config) do
      {
        "instance_url" => "https://client.my.salesforce.com",
        "sandbox" => false,
        "credentials" => {
          "client_id" => "id", "client_secret" => "secret",
          "username" => "user@test.com", "password" => "pass"
        }
      }
    end

    it "concatenates password and security_token" do
      allow(mock_client).to receive(:describe).with("Account").and_return({})
      adapter.probe

      expect(Restforce).to have_received(:new).with(hash_including(
        [REDACTED-BY-NITPICK]
      ))
    end

    it "uses sandbox login URL when sandbox is true" do
      allow(mock_client).to receive(:describe).with("Account").and_return({})
      adapter.probe

      expect(Restforce).to have_received(:new).with(hash_including(
        host: "test.salesforce.com"
      ))
    end

    it "uses production login URL when sandbox is false" do
      prod_connection = build_stubbed(:connection, db_type: "salesforce", connection_config: prod_config)
      prod_adapter = described_class.new(prod_connection)
      allow(mock_client).to receive(:describe).with("Account").and_return({})

      prod_adapter.probe

      expect(Restforce).to have_received(:new).with(hash_including(host: "login.salesforce.com"))
    end
  end
end
