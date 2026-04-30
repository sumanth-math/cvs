resource "aws_dynamodb_table" "api_records" {
  count = var.enable_api_records ? 1 : 0

  name         = local.api_records_table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "record_id"

  attribute {
    name = "record_id"
    type = "S"
  }

  attribute {
    name = "record_type"
    type = "S"
  }

  attribute {
    name = "created_at"
    type = "S"
  }

  attribute {
    name = "bucket_name"
    type = "S"
  }

  attribute {
    name = "topic_name"
    type = "S"
  }

  global_secondary_index {
    name            = "record-type-created-at"
    hash_key        = "record_type"
    range_key       = "created_at"
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = "bucket-name"
    hash_key        = "bucket_name"
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = "topic-name"
    hash_key        = "topic_name"
    projection_type = "ALL"
  }

  point_in_time_recovery {
    enabled = var.api_records_point_in_time_recovery
  }
}
