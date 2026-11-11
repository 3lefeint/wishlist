package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func upload(t *testing.T, ts *httptest.Server, as, path string, data []byte) int {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("image", "x.png")
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-Email", as)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

type testGift struct {
	ID       int  `json:"id"`
	Active   bool `json:"active"`
	HasImage bool `json:"has_image"`
	Claim    *struct {
		Name string `json:"name"`
	} `json:"claim"`
	CanEdit   bool `json:"can_edit"`
	CanDelete bool `json:"can_delete"`
	Tags      []struct {
		Name, Color string
		TextColor   string `json:"text_color"`
	} `json:"tags"`
}

func listGifts(t *testing.T, ts *httptest.Server, as, personID string) []testGift {
	t.Helper()
	code, body := call(t, ts, as, "GET", "/api/persons/"+personID+"/gifts", "")
	if code != 200 {
		t.Fatalf("list gifts as %s = %d %s", as, code, body)
	}
	var g []testGift
	if err := json.Unmarshal([]byte(body), &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGiftFlow(t *testing.T) {
	ts := newTestServer(t)
	const root, alice, bob, carol = "root@example.com", "alice@example.com", "bob@example.com", "carol@example.com"
	const outsider = "outsider@example.com"

	ids := map[string]int{}
	for _, e := range []string{root, alice, bob, carol, outsider} {
		_, body := call(t, ts, e, "GET", "/api/me", "")
		ids[e] = int(jsonField(t, body, "id"))
		if e != root {
			call(t, ts, root, "PUT", "/api/admin/users/"+itoa(ids[e])+"/status", `{"status":"approved"}`)
		}
	}
	_, body := call(t, ts, alice, "POST", "/api/circles", `{"name":"Family"}`)
	cid := itoa(int(jsonField(t, body, "id")))
	_, body = call(t, ts, alice, "POST", "/api/circles/"+cid+"/invite", "")
	var inv struct{ Token string }
	json.Unmarshal([]byte(body), &inv)
	call(t, ts, bob, "POST", "/api/invites/"+inv.Token+"/join", "")
	call(t, ts, carol, "POST", "/api/invites/"+inv.Token+"/join", "")

	personOf := func(email string) string {
		_, body := call(t, ts, alice, "GET", "/api/circles/"+cid, "")
		var d struct {
			Persons []struct {
				ID     int
				UserID *int `json:"user_id"`
			}
		}
		json.Unmarshal([]byte(body), &d)
		for _, p := range d.Persons {
			if p.UserID != nil && *p.UserID == ids[email] {
				return itoa(p.ID)
			}
		}
		t.Fatalf("no person for %s", email)
		return ""
	}
	bobList := personOf(bob)

	// validation
	for _, bad := range []string{`{"title":"  "}`, `{"title":"x","url":"javascript:alert(1)"}`, `{"title":"x","url":"ftp://h/f"}`} {
		if code, _ := call(t, ts, alice, "POST", "/api/persons/"+bobList+"/gifts", bad); code != 400 {
			t.Fatalf("%s = %d, want 400", bad, code)
		}
	}
	if code, _ := call(t, ts, outsider, "POST", "/api/persons/"+bobList+"/gifts", `{"title":"x"}`); code != 404 {
		t.Fatalf("outsider create = %d, want 404", code)
	}

	// alice puts a gift on bob's list, with tags
	code, body := call(t, ts, alice, "POST", "/api/persons/"+bobList+"/gifts",
		`{"title":"Book","comment":"hardcover","url":"https://example.com/b","tags":["Books","books"," Reading "]}`)
	if code != 201 {
		t.Fatalf("create gift = %d %s", code, body)
	}
	gid := itoa(int(jsonField(t, body, "id")))

	g := listGifts(t, ts, bob, bobList)
	if len(g) != 1 || len(g[0].Tags) != 2 || g[0].CanEdit || g[0].CanDelete {
		t.Fatalf("bob's view: %+v", g)
	}
	hexRe := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	for _, tg := range g[0].Tags {
		if !hexRe.MatchString(tg.Color) || (tg.TextColor != "#000000" && tg.TextColor != "#ffffff") {
			t.Fatalf("bad tag colours: %+v", tg)
		}
	}
	if code, body := call(t, ts, carol, "GET", "/api/circles/"+cid+"/tags", ""); code != 200 || !strings.Contains(body, "Books") {
		t.Fatalf("tags = %d %s", code, body)
	}
	if code, _ := call(t, ts, outsider, "GET", "/api/circles/"+cid+"/tags", ""); code != 404 {
		t.Fatalf("outsider tags = %d, want 404", code)
	}

	// claims: hidden from bob, visible to others, one claimer
	if code, _ := call(t, ts, bob, "PUT", "/api/gifts/"+gid+"/claim", ""); code != 409 {
		t.Fatalf("own-list claim = %d, want 409", code)
	}
	if code, _ := call(t, ts, carol, "PUT", "/api/gifts/"+gid+"/claim", ""); code != 204 {
		t.Fatalf("claim = %d", code)
	}
	if code, _ := call(t, ts, carol, "PUT", "/api/gifts/"+gid+"/claim", ""); code != 204 {
		t.Fatalf("claim again = %d", code)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/gifts/"+gid+"/claim", ""); code != 409 {
		t.Fatalf("second claimer = %d, want 409", code)
	}
	if code, _ := call(t, ts, alice, "DELETE", "/api/gifts/"+gid+"/claim", ""); code != 403 {
		t.Fatalf("foreign unclaim = %d, want 403", code)
	}
	if got := listGifts(t, ts, alice, bobList); got[0].Claim == nil || got[0].Claim.Name != "carol" {
		t.Fatalf("alice should see carol's claim: %+v", got[0])
	}
	_, raw := call(t, ts, bob, "GET", "/api/persons/"+bobList+"/gifts", "")
	if strings.Contains(raw, "carol") || !strings.Contains(raw, `"claim":null`) {
		t.Fatalf("bob must not see the claim: %s", raw)
	}

	// edit rights
	if code, _ := call(t, ts, carol, "PUT", "/api/gifts/"+gid, `{"title":"Hijacked"}`); code != 403 {
		t.Fatalf("foreign edit = %d, want 403", code)
	}
	if code, _ := call(t, ts, alice, "PUT", "/api/gifts/"+gid, `{"title":"Book 2","tags":["Books"]}`); code != 204 {
		t.Fatalf("edit = %d", code)
	}
	if g := listGifts(t, ts, alice, bobList); len(g[0].Tags) != 1 || !g[0].CanEdit {
		t.Fatalf("after edit: %+v", g[0])
	}

	// deactivate: claim stays, claimer and creator still see it, others do not
	if code, _ := call(t, ts, alice, "PUT", "/api/gifts/"+gid+"/active", `{"active":false}`); code != 204 {
		t.Fatalf("deactivate = %d", code)
	}
	if g := listGifts(t, ts, bob, bobList); len(g) != 0 {
		t.Fatalf("bob must not see a deactivated gift: %+v", g)
	}
	if g := listGifts(t, ts, carol, bobList); len(g) != 1 || g[0].Active || g[0].Claim == nil {
		t.Fatalf("claimer must still see it, deactivated, with claim: %+v", g)
	}
	if g := listGifts(t, ts, alice, bobList); len(g) != 1 || g[0].Active {
		t.Fatalf("creator must still see it: %+v", g)
	}
	call(t, ts, alice, "PUT", "/api/gifts/"+gid+"/active", `{"active":true}`)

	// images
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, image.NewNRGBA(image.Rect(0, 0, 40, 30)))
	if code := upload(t, ts, carol, "/api/gifts/"+gid+"/image", pngBuf.Bytes()); code != 403 {
		t.Fatalf("foreign upload = %d, want 403", code)
	}
	if code := upload(t, ts, outsider, "/api/gifts/"+gid+"/image", pngBuf.Bytes()); code != 404 {
		t.Fatalf("outsider upload = %d, want 404", code)
	}
	if code := upload(t, ts, alice, "/api/gifts/"+gid+"/image", []byte("nope")); code != 400 {
		t.Fatalf("garbage upload = %d, want 400", code)
	}
	if code := upload(t, ts, alice, "/api/gifts/"+gid+"/image", pngBuf.Bytes()); code != 204 {
		t.Fatalf("upload = %d", code)
	}
	for _, p := range []string{"/image", "/thumb"} {
		if code, _ := call(t, ts, bob, "GET", "/api/gifts/"+gid+p, ""); code != 200 {
			t.Fatalf("serve %s = %d", p, code)
		}
		if code, _ := call(t, ts, outsider, "GET", "/api/gifts/"+gid+p, ""); code != 404 {
			t.Fatalf("outsider %s = %d, want 404", p, code)
		}
	}
	if g := listGifts(t, ts, bob, bobList); !g[0].HasImage {
		t.Fatal("has_image must be set")
	}
	if code, _ := call(t, ts, alice, "DELETE", "/api/gifts/"+gid+"/image", ""); code != 204 {
		t.Fatalf("delete image = %d", code)
	}
	if code, _ := call(t, ts, bob, "GET", "/api/gifts/"+gid+"/image", ""); code != 404 {
		t.Fatalf("image after delete = %d, want 404", code)
	}

	// leaving a circle drops the leaver's claims; the gift stays
	if code, _ := call(t, ts, carol, "DELETE", "/api/circles/"+cid+"/members/"+itoa(ids[carol]), ""); code != 204 {
		t.Fatalf("carol leaves = %d", code)
	}
	if g := listGifts(t, ts, alice, bobList); len(g) != 1 || g[0].Claim != nil {
		t.Fatalf("claim must be gone, gift must stay: %+v", g)
	}

	// a circle administrator may delete a gift they did not create
	_, body = call(t, ts, bob, "POST", "/api/persons/"+bobList+"/gifts", `{"title":"Socks"}`)
	sid := itoa(int(jsonField(t, body, "id")))
	if code, _ := call(t, ts, alice, "DELETE", "/api/gifts/"+sid, ""); code != 204 {
		t.Fatalf("admin delete = %d", code)
	}
	if code, _ := call(t, ts, alice, "DELETE", "/api/gifts/"+gid, ""); code != 204 {
		t.Fatalf("creator delete = %d", code)
	}
	if code, _ := call(t, ts, alice, "DELETE", "/api/gifts/"+gid, ""); code != 404 {
		t.Fatalf("delete again = %d, want 404", code)
	}
}
