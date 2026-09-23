package fuzzy

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type fixture struct {
	Cases []struct {
		Name       string     `json:"name"`
		Q          string     `json:"q"`
		Targets    [][]string `json:"targets"`
		Expect     []string   `json:"expect"`
		DidYouMean string     `json:"didYouMean"`
	} `json:"cases"`
}

// The shared cases are the contract between this matcher and the
// frontend's copy of it.
func TestSharedCases(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/fixtures/fuzzy-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			targets := make([]Target, len(c.Targets))
			for i, tg := range c.Targets {
				targets[i] = NewTarget(tg[0], tg[1:]...)
			}
			ranked := Rank(NewQuery(c.Q), targets)
			got := []string{}
			for _, r := range ranked {
				got = append(got, c.Targets[r.Index][0])
			}
			if !reflect.DeepEqual(got, append([]string{}, c.Expect...)) {
				scores := []int{}
				for _, r := range ranked {
					scores = append(scores, r.Score)
				}
				t.Fatalf("q=%q got %v (scores %v), want %v", c.Q, got, scores, c.Expect)
			}
			dym := ""
			if len(ranked) > 0 && allTypos(ranked) {
				dym = ranked[0].Corrected
			}
			if dym != c.DidYouMean {
				t.Fatalf("q=%q did you mean %q, want %q", c.Q, dym, c.DidYouMean)
			}
		})
	}
}

func allTypos(r []Ranked) bool {
	for _, x := range r {
		if !x.Typo {
			return false
		}
	}
	return true
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"  José  O'Neil–Smith ": "jose o neil smith",
		"Private — Jad":         "private jad",
		"أحمد":                  "احمد",
		"فاطمة":                 "فاطمه",
		"مـحـمـد":               "محمد",
		"4.5m":                  "4 5m",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func BenchmarkRank5000(b *testing.B) {
	names := []string{"Jad Saliba", "Karim Haddad", "Lara Fares", "Nour Aoun", "Rami Khoury", "Sami Dabboussi", "Tarek Mansour"}
	targets := make([]Target, 5000)
	for i := range targets {
		targets[i] = NewTarget(names[i%len(names)]+" payment", "subscription", "70 555 666")
	}
	q := NewQuery("jda salba")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Rank(q, targets)
	}
}
