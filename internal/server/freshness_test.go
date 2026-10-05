package server

import "testing"

func TestAdminAgeAndDomainBoundaries(t *testing.T) {
	_, h, user, _, admin, csrf := testAPI(t)
	for _, path := range []string{"/admin/api/archive", "/admin/api/domains"} {
		if w := request(h, "GET", path, "", nil, ""); w.Code != 401 {
			t.Fatal(w.Code, w.Body)
		}
		if w := request(h, "GET", path, "", user, ""); w.Code != 401 {
			t.Fatal(w.Code, w.Body)
		}
		if w := request(h, "GET", path, "", admin, ""); w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if w := request(h, "POST", "/admin/api/archive", `{"days":90}`, admin, ""); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	for _, body := range []string{`{}`, `{"days":null}`, `{"days":-1}`, `{"days":1.5}`, `{"days":36501}`} {
		if w := request(h, "POST", "/admin/api/archive", body, admin, csrf); w.Code != 400 {
			t.Fatal(body, w.Code, w.Body)
		}
	}
	if w := request(h, "POST", "/admin/api/archive", `{"days":90}`, admin, csrf); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
}
