package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

type Catalog struct {
	Services       []Service                `json:"services"`
	Environments   []Environment            `json:"environments"`
	Infrastructure []InfrastructureResource `json:"infrastructure"`
}

type Service struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Owner        string            `json:"owner,omitempty"`
	Lifecycle    string            `json:"lifecycle,omitempty"`
	Tier         string            `json:"tier,omitempty"`
	Runtime      string            `json:"runtime,omitempty"`
	Repository   string            `json:"repository,omitempty"`
	HealthURL    string            `json:"healthUrl,omitempty"`
	DashboardURL string            `json:"dashboardUrl,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	Environments []string          `json:"environments,omitempty"`
	Links        []Link            `json:"links,omitempty"`
}

type Environment struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	AWSAccountID string            `json:"awsAccountId,omitempty"`
	Region       string            `json:"region,omitempty"`
	VPCID        string            `json:"vpcId,omitempty"`
	ECSCluster   string            `json:"ecsCluster,omitempty"`
	URL          string            `json:"url,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
}

type InfrastructureResource struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Provider    string            `json:"provider,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Region      string            `json:"region,omitempty"`
	ARN         string            `json:"arn,omitempty"`
	URL         string            `json:"url,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

type Link struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type StaticStore struct {
	catalog Catalog
}

func Parse(raw string) (Catalog, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return emptyCatalog(), nil
	}

	var catalog Catalog
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("PORTAL_CATALOG_JSON must be a valid catalog JSON object: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return Catalog{}, fmt.Errorf("PORTAL_CATALOG_JSON must contain one JSON object")
	}

	normalized, err := Normalize(catalog)
	if err != nil {
		return Catalog{}, fmt.Errorf("PORTAL_CATALOG_JSON: %w", err)
	}

	return normalized, nil
}

func Normalize(catalog Catalog) (Catalog, error) {
	normalized := Catalog{
		Services:       make([]Service, 0, len(catalog.Services)),
		Environments:   make([]Environment, 0, len(catalog.Environments)),
		Infrastructure: make([]InfrastructureResource, 0, len(catalog.Infrastructure)),
	}

	serviceIDs := map[string]struct{}{}
	for index, service := range catalog.Services {
		normalizedService, err := normalizeService(service)
		if err != nil {
			return Catalog{}, fmt.Errorf("services[%d]: %w", index, err)
		}
		if _, exists := serviceIDs[normalizedService.ID]; exists {
			return Catalog{}, fmt.Errorf("services[%d]: duplicate id %q", index, normalizedService.ID)
		}
		serviceIDs[normalizedService.ID] = struct{}{}
		normalized.Services = append(normalized.Services, normalizedService)
	}

	environmentIDs := map[string]struct{}{}
	for index, environment := range catalog.Environments {
		normalizedEnvironment, err := normalizeEnvironment(environment)
		if err != nil {
			return Catalog{}, fmt.Errorf("environments[%d]: %w", index, err)
		}
		if _, exists := environmentIDs[normalizedEnvironment.ID]; exists {
			return Catalog{}, fmt.Errorf("environments[%d]: duplicate id %q", index, normalizedEnvironment.ID)
		}
		environmentIDs[normalizedEnvironment.ID] = struct{}{}
		normalized.Environments = append(normalized.Environments, normalizedEnvironment)
	}

	infrastructureIDs := map[string]struct{}{}
	for index, resource := range catalog.Infrastructure {
		normalizedResource, err := normalizeInfrastructure(resource)
		if err != nil {
			return Catalog{}, fmt.Errorf("infrastructure[%d]: %w", index, err)
		}
		if _, exists := infrastructureIDs[normalizedResource.ID]; exists {
			return Catalog{}, fmt.Errorf("infrastructure[%d]: duplicate id %q", index, normalizedResource.ID)
		}
		infrastructureIDs[normalizedResource.ID] = struct{}{}
		normalized.Infrastructure = append(normalized.Infrastructure, normalizedResource)
	}

	return normalized, nil
}

func NewStaticStore(catalog Catalog) *StaticStore {
	normalized, err := Normalize(catalog)
	if err != nil {
		normalized = emptyCatalog()
	}

	return &StaticStore{catalog: normalized}
}

func (s *StaticStore) Snapshot(context.Context) Catalog {
	return s.catalog.Clone()
}

func (c Catalog) Clone() Catalog {
	clone := Catalog{
		Services:       make([]Service, len(c.Services)),
		Environments:   make([]Environment, len(c.Environments)),
		Infrastructure: make([]InfrastructureResource, len(c.Infrastructure)),
	}

	for index, service := range c.Services {
		clone.Services[index] = service
		clone.Services[index].Tags = cloneMap(service.Tags)
		clone.Services[index].Environments = cloneSlice(service.Environments)
		clone.Services[index].Links = cloneSlice(service.Links)
	}
	for index, environment := range c.Environments {
		clone.Environments[index] = environment
		clone.Environments[index].Tags = cloneMap(environment.Tags)
	}
	for index, resource := range c.Infrastructure {
		clone.Infrastructure[index] = resource
		clone.Infrastructure[index].Tags = cloneMap(resource.Tags)
	}

	return clone
}

func emptyCatalog() Catalog {
	return Catalog{
		Services:       []Service{},
		Environments:   []Environment{},
		Infrastructure: []InfrastructureResource{},
	}
}

func normalizeService(service Service) (Service, error) {
	service.ID = strings.TrimSpace(service.ID)
	if service.ID == "" {
		return Service{}, fmt.Errorf("id is required")
	}

	service.Name = strings.TrimSpace(service.Name)
	if service.Name == "" {
		return Service{}, fmt.Errorf("name is required")
	}

	service.Owner = strings.TrimSpace(service.Owner)
	service.Description = strings.TrimSpace(service.Description)
	service.Lifecycle = strings.TrimSpace(service.Lifecycle)
	service.Tier = strings.TrimSpace(service.Tier)
	service.Runtime = strings.TrimSpace(service.Runtime)
	service.Repository = strings.TrimSpace(service.Repository)
	service.HealthURL = strings.TrimSpace(service.HealthURL)
	service.DashboardURL = strings.TrimSpace(service.DashboardURL)
	service.Environments = trimStrings(service.Environments)
	service.Tags = trimMap(service.Tags)

	if err := validateOptionalHTTPURL("healthUrl", service.HealthURL); err != nil {
		return Service{}, err
	}
	if err := validateOptionalHTTPURL("dashboardUrl", service.DashboardURL); err != nil {
		return Service{}, err
	}

	for index, link := range service.Links {
		link.Title = strings.TrimSpace(link.Title)
		link.URL = strings.TrimSpace(link.URL)
		if link.Title == "" {
			return Service{}, fmt.Errorf("links[%d].title is required", index)
		}
		if err := validateOptionalHTTPURL(fmt.Sprintf("links[%d].url", index), link.URL); err != nil {
			return Service{}, err
		}
		if link.URL == "" {
			return Service{}, fmt.Errorf("links[%d].url is required", index)
		}
		service.Links[index] = link
	}

	return service, nil
}

func normalizeEnvironment(environment Environment) (Environment, error) {
	environment.ID = strings.TrimSpace(environment.ID)
	if environment.ID == "" {
		return Environment{}, fmt.Errorf("id is required")
	}

	environment.Name = strings.TrimSpace(environment.Name)
	if environment.Name == "" {
		return Environment{}, fmt.Errorf("name is required")
	}

	environment.Description = strings.TrimSpace(environment.Description)
	environment.AWSAccountID = strings.TrimSpace(environment.AWSAccountID)
	environment.Region = strings.TrimSpace(environment.Region)
	environment.VPCID = strings.TrimSpace(environment.VPCID)
	environment.ECSCluster = strings.TrimSpace(environment.ECSCluster)
	environment.URL = strings.TrimSpace(environment.URL)
	environment.Tags = trimMap(environment.Tags)

	if err := validateOptionalHTTPURL("url", environment.URL); err != nil {
		return Environment{}, err
	}

	return environment, nil
}

func normalizeInfrastructure(resource InfrastructureResource) (InfrastructureResource, error) {
	resource.ID = strings.TrimSpace(resource.ID)
	if resource.ID == "" {
		return InfrastructureResource{}, fmt.Errorf("id is required")
	}

	resource.Name = strings.TrimSpace(resource.Name)
	if resource.Name == "" {
		return InfrastructureResource{}, fmt.Errorf("name is required")
	}

	resource.Type = strings.TrimSpace(resource.Type)
	if resource.Type == "" {
		return InfrastructureResource{}, fmt.Errorf("type is required")
	}

	resource.Provider = strings.TrimSpace(resource.Provider)
	resource.Environment = strings.TrimSpace(resource.Environment)
	resource.Region = strings.TrimSpace(resource.Region)
	resource.ARN = strings.TrimSpace(resource.ARN)
	resource.URL = strings.TrimSpace(resource.URL)
	resource.Owner = strings.TrimSpace(resource.Owner)
	resource.Tags = trimMap(resource.Tags)

	if err := validateOptionalHTTPURL("url", resource.URL); err != nil {
		return InfrastructureResource{}, err
	}

	return resource, nil
}

func validateOptionalHTTPURL(field, value string) error {
	if value == "" {
		return nil
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", field, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s scheme must be http or https", field)
	}

	return nil
}

func trimStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func trimMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	trimmed := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		trimmed[key] = strings.TrimSpace(value)
	}
	if len(trimmed) == 0 {
		return nil
	}
	return trimmed
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneSlice[T any](values []T) []T {
	if len(values) == 0 {
		return nil
	}

	clone := make([]T, len(values))
	copy(clone, values)
	return clone
}
