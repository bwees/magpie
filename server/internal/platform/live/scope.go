package live

import (
	"reflect"
	"slices"

	"magpie/internal/domain"
)

// scope is what an event is about, found from the domain ID fields of its payload.
type scope struct {
	users    []domain.UserID
	requests []domain.RequestID
	titles   []domain.TitleID
	searches []domain.SearchID
}

// Payloads are flat api structs; the limit only guards against unexpected self-references.
const maxScopeDepth = 4

var (
	userIDType    = reflect.TypeFor[domain.UserID]()
	requestIDType = reflect.TypeFor[domain.RequestID]()
	titleIDType   = reflect.TypeFor[domain.TitleID]()
	searchIDType  = reflect.TypeFor[domain.SearchID]()
)

func scopeOf(payload any) scope {
	var s scope
	s.collect(reflect.ValueOf(payload), 0)
	return s
}

func (s *scope) collect(v reflect.Value, depth int) {
	if !v.IsValid() || depth > maxScopeDepth {
		return
	}
	switch v.Type() {
	case userIDType:
		s.users = appendID(s.users, domain.UserID(v.String()))
		return
	case requestIDType:
		s.requests = appendID(s.requests, domain.RequestID(v.String()))
		return
	case titleIDType:
		s.titles = appendID(s.titles, domain.TitleID(v.String()))
		return
	case searchIDType:
		s.searches = appendID(s.searches, domain.SearchID(v.String()))
		return
	}

	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			s.collect(v.Elem(), depth+1)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				s.collect(v.Field(i), depth+1)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			s.collect(v.Index(i), depth+1)
		}
	}
}

func appendID[T domain.ID](ids []T, id T) []T {
	if id == "" || slices.Contains(ids, id) {
		return ids
	}
	return append(ids, id)
}

// topicFor returns the search topic for search result events, or "" for broadcasts.
func topicFor(eventType, aggregateID string, s scope) string {
	if !searchTopicEvents[eventType] {
		return ""
	}
	if len(s.searches) > 0 {
		return SearchTopicPrefix + string(s.searches[0])
	}
	return SearchTopicPrefix + aggregateID
}
