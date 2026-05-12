// Copyright 2026 user. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"errors"
	"testing"
)

func TestSabeenManekiaImportParityCommandsAreRegistered(t *testing.T) {
	root := RootCmd()
	for _, args := range [][]string{
		{"shop", "get"},
		{"locations", "list"},
		{"collections", "list"},
		{"abandoned-checkouts", "list"},
		{"discounts", "list"},
		{"draft-orders", "list"},
	} {
		if _, _, err := root.Find(args); err != nil {
			t.Fatalf("command %v should be registered: %v", args, err)
		}
	}
}

func TestSabeenManekiaImportParityResourcesAreSyncable(t *testing.T) {
	defs := graphqlSyncDefs()
	for _, resource := range []string{
		"locations",
		"collections",
		"abandoned-checkouts",
		"discounts",
		"draft-orders",
	} {
		def, ok := defs[resource]
		if !ok {
			t.Fatalf("resource %q should be syncable", resource)
		}
		if def.Query == "" || def.FieldPath == "" || def.PageSize <= 0 {
			t.Fatalf("resource %q has incomplete sync definition: %+v", resource, def)
		}
	}
}

func TestSabeenManekiaImportParityResourcesAreInDefaultSync(t *testing.T) {
	defaults := map[string]bool{}
	for _, resource := range defaultSyncResources() {
		defaults[resource] = true
	}
	for _, resource := range []string{
		"locations",
		"collections",
		"abandoned-checkouts",
		"discounts",
		"draft-orders",
	} {
		if !defaults[resource] {
			t.Fatalf("resource %q should be included in default sync for full import parity", resource)
		}
	}
}

func TestSabeenManekiaGraphQLAccessDeniedTextIsSyncWarning(t *testing.T) {
	warning, ok := isSyncAccessWarning(errors.New("graphql: Access denied for isActive field. Required access: `read_locations` access scope."))
	if !ok {
		t.Fatalf("expected Shopify GraphQL access-denied text to be treated as a sync warning")
	}
	if warning.Reason != "forbidden" || warning.Status != 0 {
		t.Fatalf("unexpected warning classification: %+v", warning)
	}
}
