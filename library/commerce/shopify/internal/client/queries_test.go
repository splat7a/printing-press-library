// Copyright 2026 user. Licensed under Apache-2.0. See LICENSE.
package client

import (
	"strings"
	"testing"
)

func requireQueryContains(t *testing.T, name, query string, want []string) {
	t.Helper()
	for _, field := range want {
		if !strings.Contains(query, field) {
			t.Fatalf("%s should include %q for full SabeenManekia import parity\nquery:\n%s", name, field, query)
		}
	}
}

func TestSabeenManekiaImportParityQueriesIncludeOperationalFields(t *testing.T) {
	requireQueryContains(t, "ShopGetQuery", ShopGetQuery, []string{
		"enabledPresentmentCurrencies",
		"billingAddress",
		"features { storefront }",
	})
	requireQueryContains(t, "ProductsListQuery", ProductsListQuery, []string{
		"tags",
		"createdAt",
		"publishedAt",
		"totalInventory",
		"tracksInventory",
		"featuredImage{ url altText }",
		"priceRangeV2",
		"collections(first:20)",
		"seo{ title description }",
	})
	requireQueryContains(t, "CustomersListQuery", CustomersListQuery, []string{
		"displayName",
		"phone",
		"tags",
		"state",
		"verifiedEmail",
		"defaultAddress{ city province country }",
		"lastOrder{ id name createdAt }",
	})
	requireQueryContains(t, "OrdersListQuery", OrdersListQuery, []string{
		"presentmentCurrencyCode",
		"currentTotalPriceSet{ shopMoney{ amount currencyCode } presentmentMoney{ amount currencyCode } }",
		"lineItems(first:50)",
		"lineItems(first:50){ edges{ node{\n        id",
		"discountApplications(first:10)",
		"fulfillments(first:5)",
		"transactions(first:5)",
		"shippingAddress{ city province country countryCodeV2 zip }",
	})
	requireQueryContains(t, "LocationsListQuery", LocationsListQuery, []string{
		"locations(first:$first, after:$after)",
		"fulfillsOnlineOrders",
	})
	requireQueryContains(t, "CollectionsListQuery", CollectionsListQuery, []string{
		"collections(first:$first, after:$after)",
		"productsCount{ count }",
		"ruleSet{ rules{ column relation condition } }",
	})
	requireQueryContains(t, "AbandonedCheckoutsListQuery", AbandonedCheckoutsListQuery, []string{
		"abandonedCheckouts(first:$first, after:$after)",
		"abandonedCheckoutUrl",
		"lineItems(first:20)",
	})
	requireQueryContains(t, "DiscountsListQuery", DiscountsListQuery, []string{
		"codeDiscountNodes(first:$first, after:$after)",
		"DiscountCodeBasic",
		"codes(first:5)",
	})
	requireQueryContains(t, "DraftOrdersListQuery", DraftOrdersListQuery, []string{
		"draftOrders(first:$first, after:$after)",
		"totalPriceSet{ shopMoney{ amount currencyCode } }",
	})
}
