module Exports
  # Executes a single Export within a PipelineRun:
  # connect → query → map → widgets → write → attach via Active Storage → record ExportRun.
  module Executor
    module_function

    DEFAULT_MAX_UPLOAD_MB = 25

    def max_upload_bytes
      mb = ENV.fetch("MAX_UPLOAD_SIZE_MB", DEFAULT_MAX_UPLOAD_MB).to_i
      mb * 1024 * 1024
    end

    def call(export, pipeline_run:, compiled:)
      pipeline = export.pipeline

      export_run = create_export_run(export, pipeline_run: pipeline_run, compiled: compiled)
      timings = {}

      begin
        rows = timed(timings, :extraction_ms) { extract_rows(pipeline) }

        if polars_enabled?
          rows = timed(timings, :transform_ms) { Mappings::Applicator.call(rows, pipeline.mapping) }
          result = timed(timings, :write_ms) do
            polars_transform_and_write(rows, pipeline.transforms, export, export_run)
          end
        else
          rows = timed(timings, :transform_ms) do
            rows = Mappings::Applicator.call(rows, pipeline.mapping)
            apply_widgets(rows, pipeline.transforms)
          end
          result = timed(timings, :write_ms) { write_and_attach(rows, export, export_run) }
        end

        finished = Time.current
        total_ms = ((finished - export_run.started_at) * 1000).round
        rows_per_second = result[:row_count].to_f / [ total_ms / 1000.0, 0.001 ].max

        export_run.update!(
          status: "success",
          finished_at: finished,
          row_count: result[:row_count],
          bytes_written: result[:bytes_written],
          artifact_checksum: result[:artifact_checksum],
          metrics: timings.merge(
            total_ms: total_ms,
            rows_per_second: rows_per_second.round(1)
          )
        )
      rescue StandardError => e
        AppLogger.error("exports.executor", "export failed",
                        error: e, export_run: export_run.id, export: export.id, pipeline: pipeline.id)
        finished = Time.current
        total_ms = ((finished - export_run.started_at) * 1000).round

        export_run.update!(
          status: "failed",
          finished_at: finished,
          error_class: e.class.name,
          error_message: e.message,
          error_context: {},
          metrics: timings.merge(total_ms: total_ms)
        )
        raise
      end
    end

    def timed(timings, key)
      start = Process.clock_gettime(Process::CLOCK_MONOTONIC)
      result = yield
      elapsed_ms = ((Process.clock_gettime(Process::CLOCK_MONOTONIC) - start) * 1000).round
      timings[key] = elapsed_ms
      result
    end

    def polars_enabled?
      defined?(Exportee::Polars) && Exportee::Polars.enabled?
    end

    def polars_transform_and_write(rows, transforms, export, export_run)
      filename = "#{export.pipeline.name}-#{export.name}-#{Time.current.strftime('%Y%m%d%H%M%S')}.csv"
      tempfile = Tempfile.new([ "export-", ".csv" ])

      begin
        result = Transforms::DataFramePipeline.call_and_write_csv(
          rows, transforms,
          path: tempfile.path,
          mapping_fields: export.pipeline.mapping.fields
        )

        if result[:bytes_written] > max_upload_bytes
          raise ArtifactTooLarge.new(actual_bytes: result[:bytes_written], limit_bytes: max_upload_bytes)
        end

        tempfile.rewind
        export_run.artifact.attach(io: tempfile, filename: filename, content_type: "text/csv")

        dest_path = export.destination.config["path"]
        if dest_path.present?
          FileUtils.mkdir_p(File.dirname(dest_path))
          FileUtils.cp(tempfile.path, dest_path)
        end

        result
      ensure
        tempfile.close
        tempfile.unlink
      end
    end

    def extract_rows(pipeline)
      if streaming_enabled?(pipeline)
        Connections::StreamingExtractor.call_to_array(pipeline.connection, pipeline.query_text)
      else
        Connections::QueryExecutor.call(pipeline.connection, pipeline.query_text)
      end
    end

    def streaming_enabled?(pipeline)
      return false if ENV["EXPORTEE_STREAMING"] == "0"
      return false if pipeline.connection.db_type != "postgresql"

      # Pipeline-level opt-out via streaming field (future, defaults to true)
      pipeline.respond_to?(:streaming?) ? pipeline.streaming? : true
    end

    def create_export_run(export, pipeline_run:, compiled:)
      ExportRun.create!(
        org: export.org,
        source: "pipeline",
        pipeline_run: pipeline_run,
        export: export,
        compiled_definition: compiled,
        triggered_by: Current.user,
        trigger_kind: "manual",
        correlation_id: pipeline_run.correlation_id,
        status: "running",
        started_at: Time.current
      )
    end

    def apply_widgets(rows, transforms)
      return rows if transforms.blank?

      rows.filter_map do |row|
        transforms.reduce(row) do |current_row, transform|
          break nil if current_row.nil?

          widget_name = transform["widget"]
          config = transform["config"] || {}
          builtin = Widgets::Builtins.const_get(widget_name.camelize)
          builtin.call(current_row, config)
        end
      end
    end

    def write_and_attach(rows, export, export_run)
      filename = "#{export.pipeline.name}-#{export.name}-#{Time.current.strftime('%Y%m%d%H%M%S')}.csv"
      tempfile = Tempfile.new([ "export-", ".csv" ])

      begin
        result = Destinations::Writers::Csv.write(
          rows: rows,
          path: tempfile.path,
          mapping_fields: export.pipeline.mapping.fields
        )

        # Enforce upload size cap to stay under R2 free tier
        if result[:bytes_written] > max_upload_bytes
          raise ArtifactTooLarge.new(actual_bytes: result[:bytes_written], limit_bytes: max_upload_bytes)
        end

        # Attach via Active Storage
        tempfile.rewind
        export_run.artifact.attach(
          io: tempfile,
          filename: filename,
          content_type: "text/csv"
        )

        # Filesystem backup if destination path is configured
        dest_path = export.destination.config["path"]
        if dest_path.present?
          FileUtils.mkdir_p(File.dirname(dest_path))
          FileUtils.cp(tempfile.path, dest_path)
        end

        result
      ensure
        tempfile.close
        tempfile.unlink
      end
    end
  end
end
