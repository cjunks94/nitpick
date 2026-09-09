require "rails_helper"

RSpec.describe Transforms::DataFramePipeline, if: Exportee::Polars.enabled? do
  let(:rows) do
    [
      { "name" => "Alice", "email" => "alice@example.com", "ssn" => "123-45-6789", "age" => "30" },
      { "name" => "Bob", "email" => "bob@example.com", "ssn" => "987-65-4321", "age" => "25" },
      { "name" => "Carol", "email" => "carol@example.com", "ssn" => "555-12-3456", "age" => "35" }
    ]
  end

  describe ".call" do
    it "returns rows unchanged when transforms are empty" do
      result = described_class.call(rows, [])
      expect(result).to eq(rows)
    end

    it "returns empty array for empty input" do
      result = described_class.call([], [ { "widget" => "redact", "config" => { "on" => "email" } } ])
      expect(result).to eq([])
    end

    it "applies select_columns" do
      transforms = [ { "widget" => "select_columns", "config" => { "keep" => %w[name email] } } ]
      result = described_class.call(rows, transforms)
      expect(result.first.keys).to match_array(%w[name email])
      expect(result.length).to eq(3)
    end

    it "applies rename" do
      transforms = [ { "widget" => "rename", "config" => { "from" => "name", "to" => "full_name" } } ]
      result = described_class.call(rows, transforms)
      expect(result.first).to have_key("full_name")
      expect(result.first).not_to have_key("name")
    end

    it "applies redact" do
      transforms = [ { "widget" => "redact", "config" => { "on" => "ssn" } } ]
      result = described_class.call(rows, transforms)
      expect(result.map { |r| r["ssn"] }).to all(eq("[REDACTED]"))
    end

    it "applies redact with custom replacement" do
      transforms = [ { "widget" => "redact", "config" => { "on" => "ssn", "replacement" => "***" } } ]
      result = described_class.call(rows, transforms)
      expect(result.map { |r| r["ssn"] }).to all(eq("***"))
    end

    it "applies filter with eq operator" do
      transforms = [ { "widget" => "filter", "config" => { "column" => "name", "operator" => "eq", "value" => "Alice" } } ]
      result = described_class.call(rows, transforms)
      expect(result.length).to eq(1)
      expect(result.first["name"]).to eq("Alice")
    end

    it "applies mask_email" do
      transforms = [ { "widget" => "mask_email", "config" => { "on" => "email" } } ]
      result = described_class.call(rows, transforms)
      result.each do |row|
        expect(row["email"]).to include("*")
        expect(row["email"]).to include("@")
      end
    end

    it "chains multiple transforms in order" do
      transforms = [
        { "widget" => "mask_email", "config" => { "on" => "email" } },
        { "widget" => "redact", "config" => { "on" => "ssn" } },
        { "widget" => "select_columns", "config" => { "keep" => %w[name email] } }
      ]
      result = described_class.call(rows, transforms)
      expect(result.first.keys).to match_array(%w[name email])
      expect(result.first["email"]).to include("*")
      expect(result.length).to eq(3)
    end

    it "falls back to row-by-row for unknown widgets" do
      # Define a temporary custom widget
      stub_const("Widgets::Builtins::CustomThing", Module.new {
        module_function
        def call(row, _config)
          row.merge("custom" => "yes")
        end
      })

      transforms = [ { "widget" => "custom_thing", "config" => {} } ]
      result = described_class.call(rows, transforms)
      expect(result.first["custom"]).to eq("yes")
    end
  end

  describe ".call_and_write_csv" do
    let(:tempfile) { Tempfile.new(%w[test- .csv]) }
    let(:mapping_fields) { [ { "target" => "name" }, { "target" => "email" } ] }

    after { tempfile.close; tempfile.unlink }

    it "writes CSV and returns result hash" do
      transforms = [ { "widget" => "select_columns", "config" => { "keep" => %w[name email] } } ]
      result = described_class.call_and_write_csv(
        rows, transforms, path: tempfile.path, mapping_fields: mapping_fields
      )

      expect(result[:row_count]).to eq(3)
      expect(result[:bytes_written]).to be_positive
      expect(result[:artifact_checksum]).to match(/\A[0-9a-f]{64}\z/)

      csv_content = File.read(tempfile.path)
      expect(csv_content.lines.first.chomp).to eq("name,email")
      expect(csv_content.lines.length).to eq(4) # header + 3 rows
    end

    it "returns legacy result for empty rows" do
      result = described_class.call_and_write_csv(
        [], [], path: tempfile.path, mapping_fields: mapping_fields
      )
      expect(result[:row_count]).to eq(0)
    end
  end
end
