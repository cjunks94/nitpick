require "csv"

module Transforms
  # Applies widget transforms using Polars DataFrames (vectorized Rust operations)
  # instead of row-by-row Ruby iteration. 10-100x faster for large datasets.
  #
  # Toggle: only used when Exportee::Polars.enabled? is true.
  # Fallback: Exports::Executor.apply_widgets (row-by-row Ruby).
  module DataFramePipeline
    module_function

    # Accepts an array of row hashes, applies transforms, returns array of row hashes.
    # Same input/output contract as the row-by-row path for drop-in compatibility.
    def call(rows, transforms)
      return rows if rows.empty?

      df = Polars::DataFrame.new(rows)
      df = apply_transforms(df, transforms)
      df.to_hashes
    end

    # Accepts a DataFrame, applies transforms, writes CSV directly.
    # Skips the round-trip through Ruby hashes for maximum performance.
    def call_and_write_csv(rows, transforms, path:, mapping_fields:)
      return legacy_write(rows, path, mapping_fields) if rows.empty?

      df = Polars::DataFrame.new(rows)
      df = apply_transforms(df, transforms)

      headers = mapping_fields.map { |f| f["target"] }
      df = df.select(headers.select { |h| df.columns.include?(h) })

      df.write_csv(path)

      {
        row_count: df.height,
        bytes_written: File.size(path),
        artifact_url: path,
        artifact_checksum: Digest::SHA256.file(path).hexdigest
      }
    end

    def apply_transforms(df, transforms)
      return df if transforms.blank?

      transforms.each do |transform|
        widget_name = transform["widget"]
        config = transform["config"] || {}
        df = apply_one(df, widget_name, config)
      end

      df
    end

    def apply_one(df, widget_name, config)
      case widget_name
      when "select_columns"
        keep = config.fetch("keep", [])
        df.select(keep.select { |c| df.columns.include?(c) })
      when "rename"
        from = config.fetch("from")
        to = config.fetch("to")
        df.columns.include?(from) ? df.rename({ from => to }) : df
      when "filter"
        apply_filter(df, config)
      when "mask_email"
        # Polars' Rust regex engine doesn't support lookahead/lookbehind.
        # Fall back to row-by-row Ruby which handles this correctly.
        apply_row_by_row_fallback(df, "mask_email", config)
      when "redact"
        field = config.fetch("on")
        replacement = config.fetch("replacement", "[REDACTED]")
        df.columns.include?(field) ? df.with_columns(Polars.lit(replacement).alias(field)) : df
      else
        # Unknown widget — fall back to row-by-row for this one transform
        apply_row_by_row_fallback(df, widget_name, config)
      end
    end

    def apply_filter(df, config)
      column = config.fetch("column")
      operator = config.fetch("operator")
      value = config.fetch("value")
      return df unless df.columns.include?(column)

      expr = Polars.col(column)
      filter_expr = case operator
      when "eq"  then expr.eq(value)
      when "neq" then expr.ne(value)
      when "gt"  then expr.gt(value)
      when "lt"  then expr.lt(value)
      when "gte" then expr.gt_eq(value)
      when "lte" then expr.lt_eq(value)
      else return df
      end

      df.filter(filter_expr)
    end

    def apply_row_by_row_fallback(df, widget_name, config)
      builtin = Widgets::Builtins.const_get(widget_name.camelize)
      rows = df.to_hashes
      transformed = rows.filter_map { |row| builtin.call(row, config) }
      transformed.empty? ? df.clear : Polars::DataFrame.new(transformed)
    end

    def legacy_write(rows, path, mapping_fields)
      Destinations::Writers::Csv.write(rows: rows, path: path, mapping_fields: mapping_fields)
    end
  end
end
