package evaluate

import (
	"reflect"
	"testing"
)

func TestMeasureCountsLengthQuestionsAndRepeatedOpenings(t *testing.T) {
	replies := []string{
		"Here is one, and you may take it up. What do you mean?",
		"Here is one, but short. Why? And who says so?",
		"Here is one more thing to weigh before you go on and on and on and on",
		"Tell me first.",
		"Τι εννοείς; Και γιατί;",
	}
	r := Measure(replies, 12)
	if r.Replies != 5 || r.OverLimit != 2 || r.MoreThanOneQ != 2 || r.NoQuestion != 2 {
		t.Errorf("%+v", r)
	}
	if !reflect.DeepEqual(r.RepeatedStart, []string{"here is one"}) {
		t.Errorf("repeated openings: %v", r.RepeatedStart)
	}
	if r.LongestWords < r.MedianWords || r.MedianWords == 0 {
		t.Errorf("lengths: %+v", r)
	}
}

func TestMeasureIsQuietOnFewRepliesAndOnNone(t *testing.T) {
	if r := Measure(nil, 10); r.Replies != 0 || r.RepeatedStart != nil {
		t.Errorf("%+v", r)
	}
	if r := Measure([]string{"Same start here.", "Same start here again."}, 10); r.RepeatedStart != nil {
		t.Errorf("two replies cannot repeat a habit: %+v", r)
	}
}

func TestEveryRubricHasSourcedCriteriaAndNamesWhatIsContested(t *testing.T) {
	r := Rubrics()
	for _, id := range []string{"socrates", "diogenes", "plato", "aristotle"} {
		m, ok := r[id]
		if !ok || len(m.Criteria) < 8 || len(m.Foreign) < 3 {
			t.Fatalf("%s: the rubric is missing or thin", id)
		}
		contested := 0
		for _, c := range m.Criteria {
			if c.Behaviour == "" || c.Source == "" || (c.Status != "attested" && c.Status != "contested") {
				t.Errorf("%s: incomplete criterion %+v", id, c)
			}
			if c.Status == "contested" {
				contested++
			}
		}
		if contested == 0 {
			t.Errorf("%s: no criterion is marked contested, which would say that the whole picture is settled", id)
		}
	}
}
