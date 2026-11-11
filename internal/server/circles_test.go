package server

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func jsonField(t *testing.T, body, field string) float64 {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("bad json %q: %v", body, err)
	}
	v, ok := m[field].(float64)
	if !ok {
		t.Fatalf("no number %q in %s", field, body)
	}
	return v
}

func TestCircleFlow(t *testing.T) {
	ts := newTestServer(t)
	const root = "root@example.com"

	const alice, bob, carol = "alice@example.com", "bob@example.com", "carol@example.com"
	meID := func(email string) int {
		_, body := call(t, ts, email, "GET", "/api/me", "")
		return int(jsonField(t, body, "id"))
	}
	meID(root)
	ids := map[string]int{}
	for _, e := range []string{alice, bob, carol} {
		ids[e] = meID(e)
		if code, _ := call(t, ts, root, "PUT", "/api/admin/users/"+itoa(ids[e])+"/status", `{"status":"approved"}`); code != 200 {
			t.Fatalf("approve %s = %d", e, code)
		}
	}
	aID, bID, cID := itoa(ids[alice]), itoa(ids[bob]), itoa(ids[carol])

	// pending users are refused
	call(t, ts, "dave@example.com", "GET", "/api/me", "")
	if code, _ := call(t, ts, "dave@example.com", "GET", "/api/circles", ""); code != 403 {
		t.Fatalf("pending user on /api/circles = %d, want 403", code)
	}

	// create, list, isolation
	code, body := call(t, ts, alice, "POST", "/api/circles", `{"name":"Family"}`)
	if code != 201 {
		t.Fatalf("create = %d %s", code, body)
	}
	cid := int(jsonField(t, body, "id"))
	if code, _ := call(t, ts, alice, "POST", "/api/circles", `{"name":"  "}`); code != 400 {
		t.Fatalf("blank name = %d, want 400", code)
	}
	path := "/api/circles/" + itoa(cid)
	if code, _ := call(t, ts, bob, "GET", path, ""); code != 404 {
		t.Fatalf("non-member get = %d, want 404", code)
	}
	if code, body := call(t, ts, bob, "GET", "/api/circles", ""); code != 200 || strings.Contains(body, "Family") {
		t.Fatalf("bob list = %d %s", code, body)
	}

	// invite and join
	if code, _ := call(t, ts, bob, "POST", path+"/invite", ""); code != 404 {
		t.Fatalf("non-member invite = %d, want 404", code)
	}
	code, body = call(t, ts, alice, "POST", path+"/invite", "")
	if code != 201 {
		t.Fatalf("invite = %d %s", code, body)
	}
	var inv struct{ Token string }
	json.Unmarshal([]byte(body), &inv)
	if code, body := call(t, ts, bob, "GET", "/api/invites/"+inv.Token, ""); code != 200 || !strings.Contains(body, "Family") {
		t.Fatalf("invite info = %d %s", code, body)
	}
	if code, _ := call(t, ts, bob, "POST", "/api/invites/"+inv.Token+"/join", ""); code != 200 {
		t.Fatalf("join = %d", code)
	}
	if code, _ := call(t, ts, bob, "POST", "/api/invites/"+inv.Token+"/join", ""); code != 200 {
		t.Fatalf("join twice = %d", code)
	}
	if code, _ := call(t, ts, bob, "POST", path+"/invite", ""); code != 403 {
		t.Fatalf("member invite = %d, want 403", code)
	}
	if code, body := call(t, ts, bob, "GET", path, ""); code != 200 || strings.Contains(body, inv.Token) {
		t.Fatalf("member must not see the invite token: %d %s", code, body)
	}

	// renewing replaces the link; revoking switches it off
	code, body = call(t, ts, alice, "POST", path+"/invite", "")
	var inv2 struct{ Token string }
	json.Unmarshal([]byte(body), &inv2)
	if inv2.Token == inv.Token {
		t.Fatal("renewed invite must differ")
	}
	if code, _ := call(t, ts, carol, "GET", "/api/invites/"+inv.Token, ""); code != 404 {
		t.Fatalf("old invite = %d, want 404", code)
	}
	if code, _ := call(t, ts, alice, "DELETE", path+"/invite", ""); code != 204 {
		t.Fatalf("revoke = %d", code)
	}
	if code, _ := call(t, ts, carol, "POST", "/api/invites/"+inv2.Token+"/join", ""); code != 404 {
		t.Fatalf("revoked join = %d, want 404", code)
	}

	// persons and guardians
	code, body = call(t, ts, alice, "POST", path+"/persons", `{"name":"Emma"}`)
	if code != 201 {
		t.Fatalf("create person = %d %s", code, body)
	}
	pid := itoa(int(jsonField(t, body, "id")))
	if code, _ := call(t, ts, bob, "PUT", "/api/persons/"+pid, `{"name":"Emmi"}`); code != 403 {
		t.Fatalf("non-guardian rename = %d, want 403", code)
	}
	if code, _ := call(t, ts, alice, "POST", "/api/persons/"+pid+"/guardians", `{"user_id":`+cID+`}`); code != 400 {
		t.Fatalf("guardian outside circle = %d, want 400", code)
	}
	if code, _ := call(t, ts, alice, "POST", "/api/persons/"+pid+"/guardians", `{"user_id":`+bID+`}`); code != 204 {
		t.Fatalf("add guardian = %d", code)
	}
	if code, _ := call(t, ts, bob, "PUT", "/api/persons/"+pid, `{"name":"Emmi"}`); code != 204 {
		t.Fatalf("guardian rename = %d", code)
	}
	if code, body := call(t, ts, alice, "GET", path, ""); code != 200 || !strings.Contains(body, "Emmi") || !strings.Contains(body, `"guardians":[`+aID+`,`+bID+`]`) {
		t.Fatalf("detail = %d %s", code, body)
	}
	// own person entry of a member cannot be renamed through the person API
	_, body = call(t, ts, alice, "GET", path, "")
	var d struct {
		Persons []struct {
			ID     int
			UserID *int `json:"user_id"`
		}
	}
	json.Unmarshal([]byte(body), &d)
	for _, p := range d.Persons {
		if p.UserID != nil && *p.UserID == ids[alice] {
			if code, _ := call(t, ts, alice, "PUT", "/api/persons/"+itoa(p.ID), `{"name":"X"}`); code != 409 {
				t.Fatalf("rename linked person = %d, want 409", code)
			}
		}
	}

	// profile shows up for members
	for _, bad := range []string{
		`{"first_name":"","last_name":"Bobby","birthday":"1990-05-17"}`,
		`{"first_name":"Bob","last_name":"","birthday":"1990-05-17"}`,
		`{"first_name":"Bob","last_name":"Bobby","birthday":""}`,
		`{"first_name":"Bob","last_name":"Bobby","birthday":"2999-01-01"}`,
		`{"first_name":"Bob","last_name":"Bobby","birthday":"1990-05-17","favorite_color":"red"}`,
	} {
		if code, _ := call(t, ts, bob, "PUT", "/api/me", bad); code != 400 {
			t.Fatalf("invalid profile %s = %d, want 400", bad, code)
		}
	}
	if code, body := call(t, ts, bob, "PUT", "/api/me", `{"first_name":"Bob","last_name":"Bobby","birthday":"1990-05-17","favorite_color":"#3A7BD5"}`); code != 200 || !strings.Contains(body, `"profile_complete":true`) {
		t.Fatalf("set profile = %d %s", code, body)
	}
	if _, body := call(t, ts, alice, "GET", path, ""); !strings.Contains(body, "Bob Bobby") || !strings.Contains(body, `"birthday":"1990-05-17"`) || !strings.Contains(body, `"favorite_color":"#3a7bd5"`) {
		t.Fatalf("profile missing: %s", body)
	}

	// birthday of a person without an account is optional
	if _, body := call(t, ts, alice, "GET", path, ""); !strings.Contains(body, `"name":"Emmi","user_id":null,"guardians":[`+aID+`,`+bID+`],"birthday":""`) {
		t.Fatalf("person without birthday: %s", body)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/persons/"+pid, `{"name":"Emmi","birthday":"nope"}`); code != 400 {
		t.Fatalf("invalid person birthday = %d, want 400", code)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/persons/"+pid, `{"name":"Emmi","birthday":"2015-03-02"}`); code != 204 {
		t.Fatalf("set person birthday = %d", code)
	}
	if _, body := call(t, ts, alice, "GET", path, ""); !strings.Contains(body, `"birthday":"2015-03-02"`) {
		t.Fatalf("person birthday missing: %s", body)
	}

	// leaving: the only administrator cannot leave while others remain
	if code, _ := call(t, ts, alice, "DELETE", path+"/members/"+aID, ""); code != 409 {
		t.Fatalf("last admin leave = %d, want 409", code)
	}
	if code, _ := call(t, ts, bob, "DELETE", path+"/members/"+aID, ""); code != 403 {
		t.Fatalf("member removes admin = %d, want 403", code)
	}
	if code, _ := call(t, ts, bob, "DELETE", path+"/members/"+bID, ""); code != 204 {
		t.Fatalf("member leaves = %d", code)
	}
	if code, _ := call(t, ts, bob, "GET", path, ""); code != 404 {
		t.Fatalf("left member get = %d, want 404", code)
	}
	if _, body := call(t, ts, alice, "GET", path, ""); strings.Contains(body, "Bob Bobby") || strings.Contains(body, `"guardians":[`+aID+`,`+bID+`]`) {
		t.Fatalf("leaver's person entry and guardianship must be gone: %s", body)
	}

	// deleting a user hands the circle on; a lone admin's circle goes away
	_, body = call(t, ts, alice, "POST", path+"/invite", "")
	var inv3 struct{ Token string }
	json.Unmarshal([]byte(body), &inv3)
	if code, _ := call(t, ts, carol, "POST", "/api/invites/"+inv3.Token+"/join", ""); code != 200 {
		t.Fatalf("carol join = %d", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/admin/users/"+aID, ""); code != 204 {
		t.Fatalf("delete user = %d", code)
	}
	if code, body := call(t, ts, carol, "GET", path, ""); code != 200 || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("carol should be admin now: %d %s", code, body)
	}
	if code, _ := call(t, ts, carol, "DELETE", path, ""); code != 204 {
		t.Fatalf("delete circle = %d", code)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
