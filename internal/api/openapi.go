package api

const openAPIJSON = `{
  "openapi": "3.0.3",
  "info": {
    "title": "Platform Self-Service API",
    "version": "1.0.0",
    "description": "HTTP API for platform self-service workflows, including guarded S3 bucket and SNS topic provisioning, GitHub webhook automation, dependency health aggregation, and developer portal catalog metadata."
  },
  "servers": [
    {
      "url": "/"
    }
  ],
  "tags": [
    {
      "name": "Documentation"
    },
    {
      "name": "Health"
    },
    {
      "name": "S3 Buckets"
    },
    {
      "name": "SNS Topics"
    },
    {
      "name": "GitHub Webhooks"
    },
    {
      "name": "Catalog"
    }
  ],
  "paths": {
    "/openapi.json": {
      "get": {
        "tags": ["Documentation"],
        "summary": "Get the OpenAPI specification",
        "responses": {
          "200": {
            "description": "OpenAPI document",
            "content": {
              "application/vnd.oai.openapi+json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          }
        }
      }
    },
    "/swagger": {
      "get": {
        "tags": ["Documentation"],
        "summary": "Open Swagger UI",
        "responses": {
          "200": {
            "description": "Swagger UI HTML"
          }
        }
      }
    },
    "/healthz": {
      "get": {
        "tags": ["Health"],
        "summary": "Basic service health check",
        "responses": {
          "200": {
            "description": "Service is running",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/BasicHealthResponse"
                }
              }
            }
          }
        }
      }
    },
    "/v1/health-checks": {
      "get": {
        "tags": ["Health"],
        "summary": "Aggregate configured dependency health checks",
        "responses": {
          "200": {
            "description": "All configured dependencies are healthy",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/AggregateHealthResult"
                }
              }
            }
          },
          "503": {
            "description": "At least one configured dependency is unhealthy",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/AggregateHealthResult"
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          }
        }
      }
    },
    "/v1/s3-buckets": {
      "post": {
        "tags": ["S3 Buckets"],
        "summary": "Provision a guarded S3 bucket for a development team",
        "description": "Creates or configures an S3 bucket with managed naming, public access blocking, ownership controls, encryption, optional versioning, and tags. Successful responses can be recorded to DynamoDB as audit records.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/BucketRequest"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Bucket provisioned",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/BucketResult"
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          },
          "415": {
            "$ref": "#/components/responses/UnsupportedMediaType"
          },
          "500": {
            "$ref": "#/components/responses/InternalError"
          },
          "503": {
            "$ref": "#/components/responses/ServiceUnavailable"
          }
        }
      }
    },
    "/v1/sns-topics": {
      "post": {
        "tags": ["SNS Topics"],
        "summary": "Provision a guarded SNS topic for a development team",
        "description": "Creates or returns an SNS topic with managed naming, AWS-managed SNS encryption by default, optional FIFO settings, display name, and tags. Successful responses can be recorded to DynamoDB as audit records.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/SNSTopicRequest"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "SNS topic provisioned",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/SNSTopicResult"
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          },
          "415": {
            "$ref": "#/components/responses/UnsupportedMediaType"
          },
          "500": {
            "$ref": "#/components/responses/InternalError"
          },
          "503": {
            "$ref": "#/components/responses/ServiceUnavailable"
          }
        }
      }
    },
    "/v1/github/webhook": {
      "post": {
        "tags": ["GitHub Webhooks"],
        "summary": "Process GitHub webhook events",
        "description": "Processes ping, pull_request, and deployment_status events. When configured, the service verifies X-Hub-Signature-256 before processing.",
        "parameters": [
          {
            "name": "X-GitHub-Event",
            "in": "header",
            "required": true,
            "schema": {
              "type": "string",
              "example": "ping"
            }
          },
          {
            "name": "X-GitHub-Delivery",
            "in": "header",
            "required": false,
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "X-Hub-Signature-256",
            "in": "header",
            "required": false,
            "schema": {
              "type": "string",
              "example": "sha256=..."
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "additionalProperties": true
              }
            }
          }
        },
        "responses": {
          "202": {
            "description": "Webhook accepted",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/GitHubWebhookResult"
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          },
          "401": {
            "$ref": "#/components/responses/Unauthorized"
          },
          "415": {
            "$ref": "#/components/responses/UnsupportedMediaType"
          }
        }
      }
    },
    "/v1/catalog": {
      "get": {
        "tags": ["Catalog"],
        "summary": "Get the full developer portal catalog",
        "responses": {
          "200": {
            "description": "Catalog snapshot",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Catalog"
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          }
        }
      }
    },
    "/v1/catalog/services": {
      "get": {
        "tags": ["Catalog"],
        "summary": "List catalog services",
        "parameters": [
          {
            "$ref": "#/components/parameters/OwnerQuery"
          },
          {
            "$ref": "#/components/parameters/EnvironmentQuery"
          }
        ],
        "responses": {
          "200": {
            "description": "Service list",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "services": {
                      "type": "array",
                      "items": {
                        "$ref": "#/components/schemas/Service"
                      }
                    }
                  },
                  "required": ["services"]
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          }
        }
      }
    },
    "/v1/catalog/services/{id}": {
      "get": {
        "tags": ["Catalog"],
        "summary": "Get one catalog service",
        "parameters": [
          {
            "$ref": "#/components/parameters/CatalogID"
          }
        ],
        "responses": {
          "200": {
            "description": "Catalog service",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Service"
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          },
          "404": {
            "$ref": "#/components/responses/NotFound"
          }
        }
      }
    },
    "/v1/catalog/environments": {
      "get": {
        "tags": ["Catalog"],
        "summary": "List catalog environments",
        "responses": {
          "200": {
            "description": "Environment list",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "environments": {
                      "type": "array",
                      "items": {
                        "$ref": "#/components/schemas/Environment"
                      }
                    }
                  },
                  "required": ["environments"]
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          }
        }
      }
    },
    "/v1/catalog/environments/{id}": {
      "get": {
        "tags": ["Catalog"],
        "summary": "Get one catalog environment",
        "parameters": [
          {
            "$ref": "#/components/parameters/CatalogID"
          }
        ],
        "responses": {
          "200": {
            "description": "Catalog environment",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Environment"
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          },
          "404": {
            "$ref": "#/components/responses/NotFound"
          }
        }
      }
    },
    "/v1/catalog/infrastructure": {
      "get": {
        "tags": ["Catalog"],
        "summary": "List catalog infrastructure resources",
        "parameters": [
          {
            "$ref": "#/components/parameters/EnvironmentQuery"
          },
          {
            "$ref": "#/components/parameters/InfrastructureTypeQuery"
          }
        ],
        "responses": {
          "200": {
            "description": "Infrastructure resource list",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "infrastructure": {
                      "type": "array",
                      "items": {
                        "$ref": "#/components/schemas/InfrastructureResource"
                      }
                    }
                  },
                  "required": ["infrastructure"]
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          }
        }
      }
    },
    "/v1/catalog/infrastructure/{id}": {
      "get": {
        "tags": ["Catalog"],
        "summary": "Get one catalog infrastructure resource",
        "parameters": [
          {
            "$ref": "#/components/parameters/CatalogID"
          }
        ],
        "responses": {
          "200": {
            "description": "Catalog infrastructure resource",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/InfrastructureResource"
                }
              }
            }
          },
          "400": {
            "$ref": "#/components/responses/ValidationError"
          },
          "404": {
            "$ref": "#/components/responses/NotFound"
          }
        }
      }
    }
  },
  "components": {
    "parameters": {
      "CatalogID": {
        "name": "id",
        "in": "path",
        "required": true,
        "schema": {
          "type": "string",
          "minLength": 1,
          "maxLength": 128
        }
      },
      "OwnerQuery": {
        "name": "owner",
        "in": "query",
        "required": false,
        "schema": {
          "type": "string",
          "maxLength": 128
        }
      },
      "EnvironmentQuery": {
        "name": "environment",
        "in": "query",
        "required": false,
        "schema": {
          "type": "string",
          "maxLength": 128
        }
      },
      "InfrastructureTypeQuery": {
        "name": "type",
        "in": "query",
        "required": false,
        "schema": {
          "type": "string",
          "maxLength": 128
        }
      }
    },
    "responses": {
      "ValidationError": {
        "description": "Validation error",
        "content": {
          "application/json": {
            "schema": {
              "$ref": "#/components/schemas/ErrorResponse"
            }
          }
        }
      },
      "Unauthorized": {
        "description": "Missing or invalid authentication/signature",
        "content": {
          "application/json": {
            "schema": {
              "$ref": "#/components/schemas/ErrorResponse"
            }
          }
        }
      },
      "UnsupportedMediaType": {
        "description": "Content-Type must be application/json",
        "content": {
          "application/json": {
            "schema": {
              "$ref": "#/components/schemas/ErrorResponse"
            }
          }
        }
      },
      "InternalError": {
        "description": "Internal service error",
        "content": {
          "application/json": {
            "schema": {
              "$ref": "#/components/schemas/ErrorResponse"
            }
          }
        }
      },
      "ServiceUnavailable": {
        "description": "Service dependency is unavailable or not configured",
        "content": {
          "application/json": {
            "schema": {
              "$ref": "#/components/schemas/ErrorResponse"
            }
          }
        }
      },
      "NotFound": {
        "description": "Resource not found",
        "content": {
          "application/json": {
            "schema": {
              "$ref": "#/components/schemas/ErrorResponse"
            }
          }
        }
      }
    },
    "schemas": {
      "BasicHealthResponse": {
        "type": "object",
        "properties": {
          "status": {
            "type": "string",
            "example": "ok"
          }
        },
        "required": ["status"]
      },
      "BucketRequest": {
        "type": "object",
        "properties": {
          "team": {
            "type": "string",
            "example": "payments"
          },
          "environment": {
            "type": "string",
            "example": "dev"
          },
          "bucketName": {
            "type": "string",
            "description": "Optional explicit bucket name. When provided, it must start with the managed bucket prefix.",
            "example": "my-company-platform-dev-payments-dev"
          },
          "enableVersioning": {
            "type": "boolean",
            "default": true
          },
          "encryption": {
            "type": "string",
            "enum": ["AES256", "aws:kms"],
            "default": "AES256"
          },
          "kmsKeyArn": {
            "type": "string",
            "description": "Required when encryption is aws:kms."
          },
          "tags": {
            "type": "object",
            "additionalProperties": {
              "type": "string"
            },
            "example": {
              "CostCenter": "payments"
            }
          }
        },
        "required": ["team", "environment"]
      },
      "BucketResult": {
        "type": "object",
        "properties": {
          "bucketName": {
            "type": "string"
          },
          "bucketArn": {
            "type": "string"
          },
          "region": {
            "type": "string"
          },
          "versioningEnabled": {
            "type": "boolean"
          },
          "encryption": {
            "type": "string"
          },
          "tags": {
            "type": "object",
            "additionalProperties": {
              "type": "string"
            }
          }
        },
        "required": ["bucketName", "bucketArn", "region", "versioningEnabled", "encryption", "tags"]
      },
      "SNSTopicRequest": {
        "type": "object",
        "properties": {
          "team": {
            "type": "string",
            "example": "payments"
          },
          "environment": {
            "type": "string",
            "example": "dev"
          },
          "topicName": {
            "type": "string",
            "description": "Optional explicit topic name. When provided, it must start with the managed prefix. FIFO names must end with .fifo.",
            "example": "my-company-platform-dev-payments-dev"
          },
          "displayName": {
            "type": "string",
            "maxLength": 100,
            "example": "Payments events"
          },
          "fifoTopic": {
            "type": "boolean",
            "default": false
          },
          "contentBasedDeduplication": {
            "type": "boolean",
            "default": false,
            "description": "Only valid for FIFO topics."
          },
          "kmsMasterKeyId": {
            "type": "string",
            "default": "alias/aws/sns",
            "description": "SNS KMS key ID or alias. Defaults to the AWS-managed SNS key."
          },
          "tags": {
            "type": "object",
            "additionalProperties": {
              "type": "string"
            },
            "example": {
              "CostCenter": "payments"
            }
          }
        },
        "required": ["team", "environment"]
      },
      "SNSTopicResult": {
        "type": "object",
        "properties": {
          "topicName": {
            "type": "string"
          },
          "topicArn": {
            "type": "string"
          },
          "region": {
            "type": "string"
          },
          "displayName": {
            "type": "string"
          },
          "fifoTopic": {
            "type": "boolean"
          },
          "contentBasedDeduplication": {
            "type": "boolean"
          },
          "kmsMasterKeyId": {
            "type": "string"
          },
          "tags": {
            "type": "object",
            "additionalProperties": {
              "type": "string"
            }
          }
        },
        "required": ["topicName", "topicArn", "region", "fifoTopic", "contentBasedDeduplication", "kmsMasterKeyId", "tags"]
      },
      "AggregateHealthResult": {
        "type": "object",
        "properties": {
          "status": {
            "type": "string",
            "enum": ["healthy", "unhealthy"]
          },
          "checkedAt": {
            "type": "string",
            "format": "date-time"
          },
          "durationMs": {
            "type": "integer",
            "format": "int64"
          },
          "services": {
            "type": "array",
            "items": {
              "$ref": "#/components/schemas/ServiceHealthResult"
            }
          }
        },
        "required": ["status", "checkedAt", "durationMs", "services"]
      },
      "ServiceHealthResult": {
        "type": "object",
        "properties": {
          "name": {
            "type": "string"
          },
          "url": {
            "type": "string"
          },
          "status": {
            "type": "string",
            "enum": ["healthy", "unhealthy"]
          },
          "httpStatus": {
            "type": "integer"
          },
          "expectedStatus": {
            "type": "integer"
          },
          "durationMs": {
            "type": "integer",
            "format": "int64"
          },
          "error": {
            "type": "string"
          }
        },
        "required": ["name", "url", "status", "expectedStatus", "durationMs"]
      },
      "GitHubWebhookResult": {
        "type": "object",
        "properties": {
          "event": {
            "type": "string"
          },
          "deliveryId": {
            "type": "string"
          },
          "actions": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        },
        "required": ["event", "actions"]
      },
      "Catalog": {
        "type": "object",
        "properties": {
          "services": {
            "type": "array",
            "items": {
              "$ref": "#/components/schemas/Service"
            }
          },
          "environments": {
            "type": "array",
            "items": {
              "$ref": "#/components/schemas/Environment"
            }
          },
          "infrastructure": {
            "type": "array",
            "items": {
              "$ref": "#/components/schemas/InfrastructureResource"
            }
          }
        },
        "required": ["services", "environments", "infrastructure"]
      },
      "Service": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string"
          },
          "name": {
            "type": "string"
          },
          "description": {
            "type": "string"
          },
          "owner": {
            "type": "string"
          },
          "lifecycle": {
            "type": "string"
          },
          "tier": {
            "type": "string"
          },
          "runtime": {
            "type": "string"
          },
          "repository": {
            "type": "string"
          },
          "healthUrl": {
            "type": "string"
          },
          "dashboardUrl": {
            "type": "string"
          },
          "tags": {
            "type": "object",
            "additionalProperties": {
              "type": "string"
            }
          },
          "environments": {
            "type": "array",
            "items": {
              "type": "string"
            }
          },
          "links": {
            "type": "array",
            "items": {
              "$ref": "#/components/schemas/Link"
            }
          }
        },
        "required": ["id", "name"]
      },
      "Environment": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string"
          },
          "name": {
            "type": "string"
          },
          "description": {
            "type": "string"
          },
          "awsAccountId": {
            "type": "string"
          },
          "region": {
            "type": "string"
          },
          "vpcId": {
            "type": "string"
          },
          "ecsCluster": {
            "type": "string"
          },
          "url": {
            "type": "string"
          },
          "tags": {
            "type": "object",
            "additionalProperties": {
              "type": "string"
            }
          }
        },
        "required": ["id", "name"]
      },
      "InfrastructureResource": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string"
          },
          "name": {
            "type": "string"
          },
          "type": {
            "type": "string"
          },
          "provider": {
            "type": "string"
          },
          "environment": {
            "type": "string"
          },
          "region": {
            "type": "string"
          },
          "arn": {
            "type": "string"
          },
          "url": {
            "type": "string"
          },
          "owner": {
            "type": "string"
          },
          "tags": {
            "type": "object",
            "additionalProperties": {
              "type": "string"
            }
          }
        },
        "required": ["id", "name", "type"]
      },
      "Link": {
        "type": "object",
        "properties": {
          "title": {
            "type": "string"
          },
          "url": {
            "type": "string"
          }
        },
        "required": ["title", "url"]
      },
      "ErrorResponse": {
        "type": "object",
        "properties": {
          "error": {
            "type": "string"
          },
          "message": {
            "type": "string"
          },
          "fields": {
            "type": "object",
            "additionalProperties": {
              "type": "string"
            }
          }
        },
        "required": ["error", "message"]
      }
    }
  }
}`

const swaggerUIHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Platform Self-Service API Swagger</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
    <style>
      body {
        margin: 0;
        background: #f7f7f7;
      }
    </style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      window.addEventListener("load", function () {
        SwaggerUIBundle({
          url: "/openapi.json",
          dom_id: "#swagger-ui",
          deepLinking: true,
          presets: [
            SwaggerUIBundle.presets.apis
          ]
        });
      });
    </script>
  </body>
</html>`
