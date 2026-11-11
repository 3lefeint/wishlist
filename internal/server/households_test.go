package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHouseholdFlow(t *testing.T) {
	ts := newTestServer(t)
	const root = "root@example.com"
	const alice, bob, carol = "alice@example.com", "bob@example.com", "carol@example.com"

	call(t, ts, root, "GET", "/api/me", "")
	ids := map[string]int{}
	for _, e := range []string{alice, bob, carol} {
		_, body := call(t, ts, e, "GET", "/api/me", "")
		ids[e] = int(jsonField(t, body, "id"))
		if code, _ := call(t, ts, root, "PUT", "/api/admin/users/"+itoa(ids[e])+"/status", `{"status":"approved"}`); code != 200 {
			t.Fatalf("approve %s = %d", e, code)
		}
	}

	code, body := call(t, ts, alice, "POST", "/api/circles", `{"name":"Friends"}`)
	if code != 201 {
		t.Fatalf("create circle = %d %s", code, body)
	}
	path := "/api/circles/" + itoa(int(jsonField(t, body, "id")))
	code, body = call(t, ts, alice, "POST", path+"/invite", "")
	if code != 201 {
		t.Fatalf("invite = %d %s", code, body)
	}
	var inv struct{ Token string }
	json.Unmarshal([]byte(body), &inv)
	for _, e := range []string{bob, carol} {
		if code, _ := call(t, ts, e, "POST", "/api/invites/"+inv.Token+"/join", ""); code != 200 {
			t.Fatalf("%s joins = %d", e, code)
		}
	}
	code, body = call(t, ts, alice, "POST", path+"/persons", `{"name":"Kid"}`)
	if code != 201 {
		t.Fatalf("create person = %d %s", code, body)
	}
	kid := itoa(int(jsonField(t, body, "id")))

	// personOf finds the person entry of a member.
	personOf := func(email string) string {
		_, body := call(t, ts, alice, "GET", path, "")
		var d struct {
			Persons []struct {
				ID     int  `json:"id"`
				UserID *int `json:"user_id"`
			}
		}
		if err := json.Unmarshal([]byte(body), &d); err != nil {
			t.Fatal(err)
		}
		for _, p := range d.Persons {
			if p.UserID != nil && *p.UserID == ids[email] {
				return itoa(p.ID)
			}
		}
		t.Fatalf("no person for %s in %s", email, body)
		return ""
	}
	bobP, carolP := personOf(bob), personOf(carol)
	set := func(id int) string { return `{"household_id":` + itoa(id) + `}` }

	// Creating households is for the circle admin.
	if code, _ := call(t, ts, bob, "POST", path+"/households", `{"name":"Bobs"}`); code != 403 {
		t.Fatalf("member creates household = %d, want 403", code)
	}
	if code, _ := call(t, ts, alice, "POST", path+"/households", `{"name":" "}`); code != 400 {
		t.Fatalf("blank household name = %d, want 400", code)
	}
	if code, _ := call(t, ts, "dave@example.com", "POST", path+"/households", `{"name":"x"}`); code != 403 {
		t.Fatalf("pending user creates household = %d, want 403", code)
	}
	code, body = call(t, ts, alice, "POST", path+"/households", `{"name":"Smith family"}`)
	if code != 201 {
		t.Fatalf("create household = %d %s", code, body)
	}
	smith := int(jsonField(t, body, "id"))
	code, body = call(t, ts, alice, "POST", path+"/households", `{"name":"Other family"}`)
	if code != 201 {
		t.Fatalf("create household = %d %s", code, body)
	}
	other := int(jsonField(t, body, "id"))

	// Assigning: the admin moves anyone, the field must be present, the household must exist.
	if code, _ := call(t, ts, alice, "PUT", "/api/persons/"+bobP+"/household", `{}`); code != 400 {
		t.Fatalf("missing household_id = %d, want 400", code)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/persons/"+bobP+"/household", set(9999)); code != 404 {
		t.Fatalf("unknown household = %d, want 404", code)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/persons/"+bobP+"/household", set(smith)); code != 204 {
		t.Fatalf("admin assigns bob = %d, want 204", code)
	}

	// A household member manages their own household, but not others.
	if code, _ := call(t, ts, bob, "PUT", "/api/persons/"+kid+"/household", set(smith)); code != 204 {
		t.Fatalf("bob adds the kid to his household = %d, want 204", code)
	}
	if code, _ := call(t, ts, bob, "PUT", "/api/persons/"+carolP+"/household", set(smith)); code != 204 {
		t.Fatalf("bob adds carol to his household = %d, want 204", code)
	}
	if code, _ := call(t, ts, bob, "PUT", "/api/persons/"+carolP+"/household", set(other)); code != 403 {
		t.Fatalf("bob moves carol into a foreign household = %d, want 403", code)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/persons/"+carolP+"/household", set(other)); code != 204 {
		t.Fatalf("admin moves carol = %d, want 204", code)
	}
	if code, _ := call(t, ts, bob, "PUT", "/api/persons/"+carolP+"/household", `{"household_id":null}`); code != 403 {
		t.Fatalf("bob removes carol from a foreign household = %d, want 403", code)
	}
	if code, _ := call(t, ts, bob, "PUT", "/api/households/"+itoa(smith), `{"name":"Smiths"}`); code != 204 {
		t.Fatalf("member renames own household = %d, want 204", code)
	}
	if code, _ := call(t, ts, bob, "PUT", "/api/households/"+itoa(other), `{"name":"x"}`); code != 403 {
		t.Fatalf("member renames foreign household = %d, want 403", code)
	}
	if code, _ := call(t, ts, "dave@example.com", "PUT", "/api/households/"+itoa(smith), `{"name":"x"}`); code != 403 {
		t.Fatalf("pending user renames household = %d, want 403", code)
	}

	// A household of another circle is not found.
	_, body = call(t, ts, bob, "POST", "/api/circles", `{"name":"Elsewhere"}`)
	otherCircle := "/api/circles/" + itoa(int(jsonField(t, body, "id")))
	_, body = call(t, ts, bob, "POST", otherCircle+"/households", `{"name":"Foreign"}`)
	foreign := int(jsonField(t, body, "id"))
	if code, _ := call(t, ts, alice, "PUT", "/api/persons/"+kid+"/household", set(foreign)); code != 404 {
		t.Fatalf("household of another circle = %d, want 404", code)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/households/"+itoa(foreign), `{"name":"x"}`); code != 404 {
		t.Fatalf("rename in a circle the caller is not in = %d, want 404", code)
	}

	code, body = call(t, ts, carol, "GET", path, "")
	if code != 200 || !strings.Contains(body, "Smiths") || strings.Count(body, `"household_id":`+itoa(smith)) != 2 {
		t.Fatalf("detail = %d %s", code, body)
	}

	// Deleting a household keeps its persons in the circle.
	if code, _ := call(t, ts, bob, "DELETE", "/api/households/"+itoa(smith), ""); code != 403 {
		t.Fatalf("member deletes household = %d, want 403", code)
	}
	if code, _ := call(t, ts, alice, "DELETE", "/api/households/"+itoa(smith), ""); code != 204 {
		t.Fatalf("delete household = %d, want 204", code)
	}
	code, body = call(t, ts, carol, "GET", path, "")
	if code != 200 || !strings.Contains(body, "Kid") || strings.Contains(body, `"household_id":`+itoa(smith)) {
		t.Fatalf("detail after delete = %d %s", code, body)
	}

	// The operator account manages households without being a member.
	code, body = call(t, ts, root, "POST", path+"/households", `{"name":"Operator made"}`)
	if code != 201 {
		t.Fatalf("operator creates a household = %d %s", code, body)
	}
	op := int(jsonField(t, body, "id"))
	if code, _ := call(t, ts, root, "PUT", "/api/households/"+itoa(op), `{"name":"Renamed"}`); code != 204 {
		t.Fatalf("operator renames a household = %d, want 204", code)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/persons/"+carolP+"/household", set(op)); code != 204 {
		t.Fatalf("operator assigns a person = %d, want 204", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/households/"+itoa(op), ""); code != 204 {
		t.Fatalf("operator deletes a household = %d, want 204", code)
	}
}
