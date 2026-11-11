package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// The operator account manages everything without being a circle member.
func TestOperatorManagesEverythingWithoutMembership(t *testing.T) {
	ts := newTestServer(t)
	const root, alice, bob = "root@example.com", "alice@example.com", "bob@example.com"

	_, body := call(t, ts, root, "GET", "/api/me", "")
	rootID := itoa(int(jsonField(t, body, "id")))
	ids := map[string]string{}
	for _, e := range []string{alice, bob} {
		_, body = call(t, ts, e, "GET", "/api/me", "")
		ids[e] = itoa(int(jsonField(t, body, "id")))
		call(t, ts, root, "PUT", "/api/admin/users/"+ids[e]+"/status", `{"status":"approved"}`)
	}

	// Circles: seen and changed by the operator, but it cannot become a member.
	code, body := call(t, ts, alice, "POST", "/api/circles", `{"name":"Family"}`)
	if code != 201 {
		t.Fatalf("create circle = %d %s", code, body)
	}
	cid := itoa(int(jsonField(t, body, "id")))
	path := "/api/circles/" + cid
	if code, body := call(t, ts, root, "GET", "/api/circles", ""); code != 200 || !strings.Contains(body, `"Family"`) || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("operator lists circles = %d %s", code, body)
	}
	if code, _ := call(t, ts, root, "POST", "/api/circles", `{"name":"Mine"}`); code != 403 {
		t.Fatalf("operator creates a circle = %d, want 403", code)
	}
	_, body = call(t, ts, alice, "POST", path+"/invite", "")
	var inv struct{ Token string }
	json.Unmarshal([]byte(body), &inv)
	if code, _ := call(t, ts, root, "POST", "/api/invites/"+inv.Token+"/join", ""); code != 403 {
		t.Fatalf("operator joins a circle = %d, want 403", code)
	}
	if code, _ := call(t, ts, root, "POST", "/api/admin/circles/"+cid+"/members", `{"user_id":`+rootID+`,"role":"member"}`); code != 409 {
		t.Fatalf("operator added as member = %d, want 409", code)
	}
	if code, _ := call(t, ts, root, "POST", "/api/admin/circles", `{"name":"X","user_id":`+rootID+`}`); code != 409 {
		t.Fatalf("circle owned by the operator = %d, want 409", code)
	}
	if code, body := call(t, ts, root, "POST", "/api/admin/circles", `{"name":"Created","user_id":`+ids[bob]+`}`); code != 201 || !strings.Contains(body, `"admin"`) {
		t.Fatalf("admin creates a circle = %d %s", code, body)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/admin/circles/"+cid, `{"name":"Relatives"}`); code != 204 {
		t.Fatalf("admin renames a circle = %d, want 204", code)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/admin/circles/999", `{"name":"x"}`); code != 404 {
		t.Fatalf("rename unknown circle = %d, want 404", code)
	}

	// Not visible: the operator is in no member list and no guardian list.
	code, body = call(t, ts, root, "GET", path, "")
	if code != 200 || !strings.Contains(body, `"invite":{`) {
		t.Fatalf("operator reads circle = %d %s", code, body)
	}
	if code, body = call(t, ts, alice, "GET", path, ""); code != 200 || strings.Contains(body, "root@example.com") || strings.Contains(body, `"name":"root"`) {
		t.Fatalf("operator visible to members = %d %s", code, body)
	}

	// Persons without an account.
	code, body = call(t, ts, root, "POST", path+"/persons", `{"name":"Kid"}`)
	if code != 201 || !strings.Contains(body, `"guardians":[]`) {
		t.Fatalf("operator creates a person = %d %s", code, body)
	}
	pid := itoa(int(jsonField(t, body, "id")))
	if code, _ := call(t, ts, root, "PUT", "/api/persons/"+pid, `{"name":"Kid Two","birthday":""}`); code != 204 {
		t.Fatalf("operator renames a person = %d, want 204", code)
	}
	if code, _ := call(t, ts, root, "POST", "/api/persons/"+pid+"/guardians", `{"user_id":`+rootID+`}`); code != 400 {
		t.Fatalf("operator as guardian = %d, want 400", code)
	}
	if code, _ := call(t, ts, root, "POST", "/api/persons/"+pid+"/guardians", `{"user_id":`+ids[alice]+`}`); code != 204 {
		t.Fatalf("operator adds a guardian = %d, want 204", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/persons/"+pid+"/guardians/"+ids[alice], ""); code != 204 {
		t.Fatalf("operator removes a guardian = %d, want 204", code)
	}
	if code, body := call(t, ts, root, "GET", "/api/persons/"+pid, ""); code != 200 || !strings.Contains(body, "Kid Two") {
		t.Fatalf("operator reads a person = %d %s", code, body)
	}

	// Gifts: create, edit (also those of others), deactivate, withdraw claims, delete; never claim.
	code, body = call(t, ts, root, "POST", "/api/persons/"+pid+"/gifts", `{"title":"Ball","comment":"","url":"","tags":["toys"]}`)
	if code != 201 {
		t.Fatalf("operator creates a gift = %d %s", code, body)
	}
	gid := itoa(int(jsonField(t, body, "id")))
	if code, _ := call(t, ts, alice, "POST", path+"/persons", `{"name":"Other"}`); code != 201 {
		t.Fatalf("alice creates a person = %d", code)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/gifts/"+gid+"/claim", ""); code != 200 && code != 204 {
		t.Fatalf("alice claims = %d", code)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/gifts/"+gid+"/claim", ""); code != 403 {
		t.Fatalf("operator claims = %d, want 403", code)
	}
	if code, body := call(t, ts, root, "GET", "/api/persons/"+pid+"/gifts", ""); code != 200 || !strings.Contains(body, "Ball") || strings.Contains(body, `"claim":{`) {
		t.Fatalf("operator lists gifts = %d %s", code, body)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/gifts/"+gid, `{"title":"Big ball","comment":"","url":"","tags":["toys","sport"]}`); code != 204 {
		t.Fatalf("operator edits a gift of someone else = %d, want 204", code)
	}
	if code, body := call(t, ts, root, "GET", "/api/circles/"+cid+"/tags", ""); code != 200 || !strings.Contains(body, "sport") {
		t.Fatalf("operator lists tags = %d %s", code, body)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/gifts/"+gid+"/claim", ""); code != 204 {
		t.Fatalf("operator withdraws a claim = %d, want 204", code)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/gifts/"+gid+"/active", `{"active":false}`); code != 204 {
		t.Fatalf("operator deactivates a gift = %d, want 204", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/gifts/"+gid, ""); code != 204 {
		t.Fatalf("operator deletes a gift = %d, want 204", code)
	}

	// Circle invites and removal of persons and circles.
	if code, _ := call(t, ts, root, "POST", path+"/invite", ""); code != 201 {
		t.Fatalf("operator creates an invite = %d, want 201", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/persons/"+pid, ""); code != 204 {
		t.Fatalf("operator deletes a person = %d, want 204", code)
	}
	if code, _ := call(t, ts, root, "DELETE", path, ""); code != 204 {
		t.Fatalf("operator deletes a circle = %d, want 204", code)
	}

	// Strangers still see nothing.
	if code, _ := call(t, ts, bob, "GET", path, ""); code != 404 {
		t.Fatalf("bob reads a deleted circle = %d, want 404", code)
	}
}
