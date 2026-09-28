package service

import (
	"context"
	"strings"
)

// searchPersonAndMediaItems composes two paginated streams without truncating
// totals or reordering subsequent pages. People are returned before media.
func (e *EmbyService) searchPersonAndMediaItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	p.Limit = min(p.Limit, 500)
	personParams := p
	personParams.Limit = min(p.Limit, 500)
	people, err := e.Persons(ctx, personParams)
	if err != nil {
		return nil, err
	}
	totalPeople := embyEnvelopeCount(people)
	items := people["Items"].([]map[string]any)
	types := make([]string, 0, len(p.IncludeItemTypes))
	for _, typ := range p.IncludeItemTypes {
		if !strings.EqualFold(strings.TrimSpace(typ), "Person") {
			types = append(types, typ)
		}
	}
	if len(types) == 0 {
		return people, nil
	}
	mediaParams := p
	mediaParams.preserveSearchTypes = true
	mediaParams.IncludeItemTypes = types
	mediaParams.StartIndex = max(0, p.StartIndex-totalPeople)
	mediaParams.Limit = max(1, p.Limit-len(items))
	media, err := e.Items(ctx, mediaParams)
	if err != nil {
		return nil, err
	}
	if len(items) < p.Limit {
		items = append(items, media["Items"].([]map[string]any)...)
	}
	return map[string]any{"Items": items, "TotalRecordCount": totalPeople + embyEnvelopeCount(media), "StartIndex": p.StartIndex}, nil
}

func embyEnvelopeCount(envelope map[string]any) int {
	switch value := envelope["TotalRecordCount"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}
