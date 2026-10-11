package app

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestMessagesMatchTheEventCatalog(t *testing.T) {
	want := catalogMessages(t)
	got := map[string]bool{}
	for _, m := range Messages() {
		if _, dup := got[m.Type()]; dup {
			t.Errorf("%s is registered twice", m.Type())
		}
		got[m.Type()] = m.Durable()
	}

	for _, missing := range slices.Sorted(maps.Keys(want)) {
		if _, ok := got[missing]; !ok {
			t.Errorf("%s is in the event catalog but has no struct", missing)
		}
	}
	for _, extra := range slices.Sorted(maps.Keys(got)) {
		durable, ok := want[extra]
		if !ok {
			t.Errorf("%s is registered but not in the event catalog", extra)
			continue
		}
		if got[extra] != durable {
			t.Errorf("%s durable = %v, catalog says %v", extra, got[extra], durable)
		}
	}
}

func TestMessageTypesNameTheirStruct(t *testing.T) {
	for _, m := range Messages() {
		name := reflect.TypeOf(m).Name()
		if _, suffix, _ := strings.Cut(m.Type(), messageTypeSeparator); suffix != name {
			t.Errorf("%T has type %q; the name after the context must be %q", m, m.Type(), name)
		}
	}
}
