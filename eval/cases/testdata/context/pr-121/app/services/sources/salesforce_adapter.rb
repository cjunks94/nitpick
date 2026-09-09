module Sources
  # Salesforce source adapter using the Restforce gem.
  # Supports OAuth2 username-password flow for authentication,
  # SOQL queries for extraction, and describe calls for introspection.
  #
  # Connection config expected:
  #   instance_url: "https://client.my.salesforce.com"
  #   api_version: "61.0" (optional, defaults to Restforce default)
  #   sandbox: false (optional, switches login URL)
  #
  # Credentials resolved at runtime via from_secret:
  #   client_id, client_secret, username, password, security_token
  class SalesforceAdapter < BaseAdapter
    def probe
      client.describe("Account")
      { ok: true, error: nil }
    rescue Restforce::Error, Faraday::Error => e
      { ok: false, error: e.message }
    end

    def introspect_schema
      # List queryable standard + custom objects, then describe each
      sobjects = client.describe
      queryable = sobjects.select { |s| s["queryable"] }

      queryable.map do |sobject|
        describe = client.describe(sobject["name"])
        {
          table: sobject["name"],
          columns: describe["fields"].map do |f|
            {
              name: f["name"],
              type: f["type"],
              nullable: f["nillable"]
            }
          end
        }
      end
    rescue Restforce::Error, Faraday::Error => e
      raise Sources::AdapterError, "Salesforce introspection failed: #{e.message}"
    end

    def extract(query)
      rows = []
      client.query(query).each { |record| rows << normalize_record(record) }
      rows
    rescue Restforce::Error, Faraday::Error => e
      raise Sources::AdapterError, "Salesforce query failed: #{e.message}"
    end

    # Salesforce doesn't have a COPY-like streaming protocol.
    # query_all includes archived/deleted records when needed.
    def extract_streaming(query)
      extract(query)
    end

    private

    def client
      @client ||= Restforce.new(
        username: credentials["username"],
        password: password_with_token,
        client_id: credentials["client_id"],
        client_secret: credentials["client_secret"],
        instance_url: config["instance_url"],
        api_version: config.fetch("api_version", nil),
        host: config["sandbox"] ? "test.salesforce.com" : "login.salesforce.com",
        authentication_retries: 1,
        request_headers: { "Sforce-Query-Options" => "batchSize=2000" }
      ).tap(&:authenticate!)
    end

    def credentials
      @credentials ||= connection.connection_config.fetch("credentials", config)
    end

    def password_with_token
      [ credentials["password"], credentials["security_token"] ].compact.join
    end

    # Restforce returns Restforce::SObject (Hashie::Mash subclass).
    # Normalize to plain hash and strip Salesforce metadata.
    def normalize_record(record)
      record.to_hash.except("attributes")
    end
  end
end
