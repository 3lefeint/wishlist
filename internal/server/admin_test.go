package server

import (
	"regexp"
	"strings"
	"testing"
)

func TestAdminContent(t *testing.T) {
	ts := newTestServer(t)
	const root, alice, bob = "root@example.com", "alice@example.com", "bob@example.com"
	call(t, ts, root, "GET", "/api/me", "")
	ids := map[string]string{}
	for _, e := range []string{alice, bob} {
		_, body := call(t, ts, e, "GET", "/api/me", "")
		ids[e] = itoa(int(jsonField(t, body, "id")))
		if code, _ := call(t, ts, root, "PUT", "/api/admin/users/"+ids[e]+"/status", `{"status":"approved"}`); code != 200 {
			t.Fatalf("approve %s = %d", e, code)
		}
	}

	// alice has a circle with a person and a claimed gift of bob's
	_, body := call(t, ts, alice, "POST", "/api/circles", `{"name":"Family"}`)
	cid := itoa(int(jsonField(t, body, "id")))
	call(t, ts, alice, "POST", "/api/circles/"+cid+"/persons", `{"name":"Emma"}`)
	_, body = call(t, ts, alice, "GET", "/api/circles/"+cid, "")
	pid := regexpFirst(t, body, `"id":(\d+),"name":"Emma"`)
	code, body := call(t, ts, alice, "POST", "/api/persons/"+pid+"/gifts", `{"title":"Bike","comment":"red","url":"","tags":[]}`)
	if code != 201 {
		t.Fatalf("create gift = %d %s", code, body)
	}
	gid := itoa(int(jsonField(t, body, "id")))

	// only administrators get in
	for _, p := range []string{"/api/admin/circles", "/api/admin/circles/" + cid, "/api/admin/circles/" + cid + "/gifts"} {
		if code, _ := call(t, ts, alice, "GET", p, ""); code != 403 {
			t.Fatalf("non-admin GET %s = %d, want 403", p, code)
		}
	}
	if code, _ := call(t, ts, alice, "DELETE", "/api/admin/circles/"+cid, ""); code != 403 {
		t.Fatalf("non-admin delete = %d, want 403", code)
	}

	if code, body := call(t, ts, root, "GET", "/api/admin/circles", ""); code != 200 || !strings.Contains(body, `"name":"Family","member_count":1,"gift_count":1`) {
		t.Fatalf("circle list = %d %s", code, body)
	}
	if code, body := call(t, ts, root, "GET", "/api/admin/circles/"+cid, ""); code != 200 || !strings.Contains(body, "alice") || strings.Contains(body, `"invite":{`) {
		t.Fatalf("circle detail = %d %s", code, body)
	}
	if code, _ := call(t, ts, root, "GET", "/api/admin/circles/999", ""); code != 404 {
		t.Fatalf("missing circle = %d, want 404", code)
	}

	// gifts are visible, claims are not
	call(t, ts, bob, "GET", "/api/me", "")
	if code, _ := call(t, ts, root, "POST", "/api/admin/circles/"+cid+"/members", `{"user_id":`+ids[bob]+`,"role":"member"}`); code != 204 {
		t.Fatalf("add member = %d", code)
	}
	if code, _ := call(t, ts, bob, "PUT", "/api/gifts/"+gid+"/claim", ""); code != 204 {
		t.Fatalf("claim = %d", code)
	}
	code, body = call(t, ts, root, "GET", "/api/admin/circles/"+cid+"/gifts", "")
	if code != 200 || !strings.Contains(body, `"title":"Bike"`) || !strings.Contains(body, `"person_name":"Emma"`) || strings.Contains(strings.ToLower(body), "claim") || strings.Contains(body, "bob") {
		t.Fatalf("gift list = %d %s", code, body)
	}

	// memberships
	if code, _ := call(t, ts, root, "POST", "/api/admin/circles/"+cid+"/members", `{"user_id":`+ids[bob]+`,"role":"member"}`); code != 409 {
		t.Fatalf("add twice = %d, want 409", code)
	}
	if code, _ := call(t, ts, root, "POST", "/api/admin/circles/"+cid+"/members", `{"user_id":`+ids[bob]+`,"role":"boss"}`); code != 400 {
		t.Fatalf("bad role = %d, want 400", code)
	}
	if code, _ := call(t, ts, root, "POST", "/api/admin/circles/"+cid+"/members", `{"user_id":999,"role":"member"}`); code != 404 {
		t.Fatalf("unknown user = %d, want 404", code)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/admin/circles/"+cid+"/members/"+ids[alice], `{"role":"member"}`); code != 409 {
		t.Fatalf("demote last admin = %d, want 409", code)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/admin/circles/"+cid+"/members/"+ids[bob], `{"role":"admin"}`); code != 204 {
		t.Fatalf("promote = %d", code)
	}
	if code, _ := call(t, ts, root, "PUT", "/api/admin/circles/"+cid+"/members/"+ids[alice], `{"role":"member"}`); code != 204 {
		t.Fatalf("demote with another admin = %d", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/admin/circles/"+cid+"/members/"+ids[bob], ""); code != 409 {
		t.Fatalf("remove last admin = %d, want 409", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/admin/circles/"+cid+"/members/"+ids[alice], ""); code != 204 {
		t.Fatalf("remove member = %d", code)
	}
	if code, _ := call(t, ts, alice, "GET", "/api/circles/"+cid, ""); code != 404 {
		t.Fatalf("removed member still sees the circle: %d", code)
	}

	// gifts: deactivate, delete
	if code, _ := call(t, ts, root, "PUT", "/api/admin/gifts/"+gid+"/active", `{"active":false}`); code != 204 {
		t.Fatalf("deactivate = %d", code)
	}
	if _, body := call(t, ts, root, "GET", "/api/admin/circles/"+cid+"/gifts", ""); !strings.Contains(body, `"active":false`) {
		t.Fatalf("gift still active: %s", body)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/admin/gifts/"+gid, ""); code != 204 {
		t.Fatalf("delete gift = %d", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/admin/gifts/"+gid, ""); code != 404 {
		t.Fatalf("delete gift twice = %d, want 404", code)
	}

	// circles
	if code, _ := call(t, ts, root, "DELETE", "/api/admin/circles/"+cid, ""); code != 204 {
		t.Fatalf("delete circle = %d", code)
	}
	if code, _ := call(t, ts, root, "DELETE", "/api/admin/circles/"+cid, ""); code != 404 {
		t.Fatalf("delete circle twice = %d, want 404", code)
	}
}

func regexpFirst(t *testing.T, s, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("%q not found in %s", pattern, s)
	}
	return m[1]
}
