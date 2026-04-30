package catalog

import "testing"

func TestParseCatalog(t *testing.T) {
	catalog, err := Parse(`{
		"services": [
			{
				"id": "platform-api",
				"name": "Platform API",
				"owner": "platform",
				"repository": "https://github.com/sumanth-math/cvs",
				"environments": ["dev"],
				"links": [{"title": "Runbook", "url": "https://example.com/runbook"}]
			}
		],
		"environments": [
			{"id": "dev", "name": "Development", "region": "us-east-1"}
		],
		"infrastructure": [
			{"id": "platform-alb", "name": "Platform ALB", "type": "alb", "provider": "aws", "environment": "dev"}
		]
	}`)
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}

	if len(catalog.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(catalog.Services))
	}
	if catalog.Services[0].ID != "platform-api" {
		t.Fatalf("unexpected service id: %q", catalog.Services[0].ID)
	}
	if len(catalog.Environments) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(catalog.Environments))
	}
	if len(catalog.Infrastructure) != 1 {
		t.Fatalf("expected 1 infrastructure resource, got %d", len(catalog.Infrastructure))
	}
}

func TestParseEmptyCatalog(t *testing.T) {
	catalog, err := Parse("")
	if err != nil {
		t.Fatalf("parse empty catalog: %v", err)
	}
	if len(catalog.Services) != 0 || len(catalog.Environments) != 0 || len(catalog.Infrastructure) != 0 {
		t.Fatalf("expected empty catalog, got %+v", catalog)
	}
}

func TestParseCatalogRejectsDuplicateServiceIDs(t *testing.T) {
	_, err := Parse(`{
		"services": [
			{"id": "platform-api", "name": "Platform API"},
			{"id": "platform-api", "name": "Duplicate"}
		]
	}`)
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestParseCatalogRejectsTrailingJSON(t *testing.T) {
	_, err := Parse(`{"services": []} {"services": []}`)
	if err == nil {
		t.Fatal("expected trailing JSON error")
	}
}

func TestParseCatalogRejectsInvalidURL(t *testing.T) {
	_, err := Parse(`{
		"services": [
			{"id": "platform-api", "name": "Platform API", "healthUrl": "ftp://example.com/health"}
		]
	}`)
	if err == nil {
		t.Fatal("expected invalid URL error")
	}
}
