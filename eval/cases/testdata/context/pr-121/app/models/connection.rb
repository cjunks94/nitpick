class Connection < ApplicationRecord
  include Auditable
  include SoftDeletable
  include UuidIdentified
  include OrgScoped

  has_encrypted :connection_config, type: :json

  enum :db_type, { mysql: "mysql", postgresql: "postgresql", sqlite: "sqlite", salesforce: "salesforce" }
  enum :connection_status,
       { unknown: "unknown", ok: "ok", error: "error" },
       prefix: :connection

  validates :name, presence: true, uniqueness: { scope: :org_id, case_sensitive: false }
  validates :db_type, presence: true
  validates :query_timeout, numericality: { only_integer: true, greater_than: 0 }
end
