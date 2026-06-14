package okf

import "testing"

func TestKnowledgeBundleOperations(t *testing.T) {
	t.Parallel()

	bundle := NewBundle("demo")
	users := NewConcept("api", "Users")
	users.Tags = []string{"go", "production"}
	users.Resource = "api/users"
	orders := NewConcept("api", "Orders")
	orders.Tags = []string{"go"}
	orders.Resource = "api/orders"
	usersCopy := NewConcept("doc", "Users Copy")
	usersCopy.Tags = []string{"docs"}
	usersCopy.Resource = "api/users"

	bundle.AddConcept(users)
	bundle.AddConcept(orders)
	bundle.AddConcept(usersCopy)

	if got := bundle.GetConcept("Users"); got != users {
		t.Fatalf("GetConcept returned %#v, want Users", got)
	}
	if got := bundle.FilterByType("api"); len(got) != 2 {
		t.Fatalf("FilterByType returned %d concepts, want 2", len(got))
	}
	if got := bundle.FilterByTag("go"); len(got) != 2 {
		t.Fatalf("FilterByTag returned %d concepts, want 2", len(got))
	}
	if got := bundle.FilterByResource("api/users"); len(got) != 2 {
		t.Fatalf("FilterByResource returned %d concepts, want 2", len(got))
	}
	if got := bundle.RelatedConcepts(users); len(got) != 2 {
		t.Fatalf("RelatedConcepts returned %d concepts, want 2", len(got))
	}
	if !bundle.RemoveConcept("Orders") || bundle.GetConcept("Orders") != nil {
		t.Fatal("RemoveConcept did not remove Orders")
	}
}
