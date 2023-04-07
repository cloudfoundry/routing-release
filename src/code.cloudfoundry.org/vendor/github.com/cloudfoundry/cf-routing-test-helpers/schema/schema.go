package schema

type Metadata struct {
	Guid string
}

type Resource struct {
	Metadata Metadata
}

type ListResponse struct {
	TotalResults int `json:"total_results"`
	Resources    []Resource
}

type AppResource struct {
	Metadata struct {
		Url string
	}
}

type AppsResponse struct {
	Resources []AppResource
}
type Stat struct {
	Stats struct {
		Host string
		Port int
	}
}
type StatsResponse map[string]Stat

type RouteResource struct {
	Entity struct {
		Port uint16
	}
}

type OrgResource struct {
	Entity struct {
		QuotaDefinitionUrl string `json:"quota_definition_url"`
	}
}
