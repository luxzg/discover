package urlnorm

import "testing"

func TestIdentityQueries(t *testing.T) {
	a, ha, _, err := Normalize("https://EXAMPLE.com/news?id=1&sig=a%2Bb&x=2&x=3&utm_source=x#section")
	if err != nil {
		t.Fatal(err)
	}
	if a != "https://example.com/news?id=1&sig=a%2Bb&x=2&x=3" {
		t.Fatal(a)
	}
	_, hb, _, _ := Normalize("https://example.com/news?id=2&sig=a%2Bb&x=2&x=3")
	if ha == hb {
		t.Fatal("identity params collapsed")
	}
	_, hc, _, _ := Normalize("https://example.com/news?id=1&sig=a%2Bb&x=2&x=3")
	if ha != hc {
		t.Fatal("tracking params not removed")
	}
	for _, s := range []string{"javascript:alert(1)", "https://user:password@example.com/", "/relative"} {
		if _, _, _, err := Normalize(s); err == nil {
			t.Fatal(s)
		}
	}
}
