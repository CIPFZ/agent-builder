package memory

import (
	"sort"
	"strings"
)

func Rank(records []Record, bodies map[string]string, req SearchRequest) []SearchResult {
	terms := keywordSet(req.Query)
	typeFilter := set(req.Types)
	tagFilter := set(req.Tags)
	results := make([]SearchResult, 0, len(records))
	for _, record := range records {
		if !record.Enabled || record.DeletedAt != "" {
			continue
		}
		if len(typeFilter) > 0 {
			if _, ok := typeFilter[record.Type]; !ok {
				continue
			}
		}
		if len(tagFilter) > 0 && !hasAnyTag(record.Tags, tagFilter) {
			continue
		}
		body := bodies[record.ID]
		score := 0
		haystack := keywordSet(strings.Join([]string{record.Title, record.Description, strings.Join(record.Tags, " "), Preview(body, 1200)}, " "))
		for term := range terms {
			if _, ok := haystack[term]; ok {
				score += 10
			}
		}
		for _, tag := range record.Tags {
			if _, ok := terms[tag]; ok {
				score += 8
			}
		}
		if strings.TrimSpace(req.Query) == "" {
			score = 1
		}
		if score <= 0 {
			continue
		}
		results = append(results, SearchResult{
			Record:          record,
			Score:           score,
			SelectionReason: "keyword_overlap",
			Content:         body,
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Record.UpdatedAt > results[j].Record.UpdatedAt
	})
	limit := req.Limit
	if limit <= 0 || limit > 5 {
		limit = 5
	}
	if req.TokenBudget > 0 {
		total := 0
		filtered := results[:0]
		for _, result := range results {
			if total+result.Record.TokenEstimate > req.TokenBudget {
				continue
			}
			total += result.Record.TokenEstimate
			filtered = append(filtered, result)
			if len(filtered) >= limit {
				break
			}
		}
		return append([]SearchResult(nil), filtered...)
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func keywordSet(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, field := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && (r < 0x4e00 || r > 0x9fff)
	}) {
		field = strings.TrimSpace(field)
		if len([]rune(field)) < 2 {
			continue
		}
		out[field] = struct{}{}
	}
	return out
}

func set(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func hasAnyTag(tags []string, filter map[string]struct{}) bool {
	for _, tag := range tags {
		if _, ok := filter[strings.ToLower(strings.TrimSpace(tag))]; ok {
			return true
		}
	}
	return false
}
