package suggest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"socrates/internal/llm"
)

var bg = context.Background()

func client(steps ...llm.MockStep) (*llm.Mock, Completer) {
	m := llm.NewMock(steps...)
	return m, llm.NewClientWith(m, llm.Settings{FastModel: "f", ReplyModel: "r"})
}

var talk = []llm.Message{{Role: "user", Content: "Is it ever okay to lie?"}, {Role: "assistant", Content: "What do you mean by a lie?"}}

const good = `{"suggestions":["A lie is saying what you know is false.","If a friend were hurt by the truth I might lie.","I think lying always costs trust.","I really don't know where to start."]}`

func TestFourSuggestionsInTheUsersVoiceAreReadFromTheKeyedAnswer(t *testing.T) {
	m, c := client(llm.Say(good))
	r, err := Generate(bg, c, talk, nil)
	if err != nil || r.Fallback || len(r.Suggestions) != 4 || r.Suggestions[3] != "I really don't know where to start." {
		t.Fatalf("%+v %v", r, err)
	}
	req := m.Requests[0]
	if req.Role != llm.RoleFast || !req.JSON || !strings.Contains(req.Messages[0].Content, "What do you mean by a lie?") {
		t.Fatalf("%+v", req)
	}
	if !strings.Contains(req.System, "first person") || !strings.Contains(req.System, "do not answer for the philosopher") {
		t.Fatal("the instruction lacks the voice or the limit")
	}
}

func TestChatterAroundTheJSONIsTolerated(t *testing.T) {
	_, c := client(llm.Say("Sure! Here you go:\n```json\n" + good + "\n```\nHope that helps."))
	r, err := Generate(bg, c, talk, nil)
	if err != nil || r.Fallback || len(r.Suggestions) != 4 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestUnusableAnswersAreTriedAgainThenTheGeneralListIsUsedAndSaysSo(t *testing.T) {
	m, c := client(llm.Say("1. one\n2. two"), llm.Say(`{"suggestions":["only one"]}`))
	r, err := Generate(bg, c, talk, nil)
	if err != nil || !r.Fallback || len(r.Suggestions) != 4 || len(m.Requests) != 2 {
		t.Fatalf("%+v %v after %d calls", r, err, len(m.Requests))
	}
	_, c = client(llm.Say("not json"), llm.Say(good))
	if r, _ := Generate(bg, c, talk, nil); r.Fallback {
		t.Fatal("the second try was not used")
	}
}

func TestOverlongRepeatedAndAlreadyOfferedSuggestionsAreDropped(t *testing.T) {
	long := strings.Repeat("a", maxChars+1)
	raw := `{"suggestions":["Same one.","same   one.","` + long + `","","Offered before.","Fresh one A.","Fresh one B.","Fresh one C."]}`
	_, c := client(llm.Say(raw))
	r, _ := Generate(bg, c, talk, []string{"offered BEFORE."})
	if r.Fallback || len(r.Suggestions) != 4 || r.Suggestions[0] != "Same one." || r.Suggestions[1] != "Fresh one A." {
		t.Fatalf("%+v", r)
	}
}

func TestAskingAgainTellsTheModelWhatNotToRepeat(t *testing.T) {
	m, c := client(llm.Say(good))
	Generate(bg, c, talk, []string{"First set A.", "First set B."})
	if p := m.Requests[0].Messages[0].Content; !strings.Contains(p, "already offered") || !strings.Contains(p, "First set B.") {
		t.Fatal(p)
	}
}

func TestTheGeneralListAlsoHonoursWhatWasAlreadyOffered(t *testing.T) {
	_, c := client(llm.Say("x"), llm.Say("y"))
	r, _ := Generate(bg, c, talk, Fallback[:2])
	if !r.Fallback || len(r.Suggestions) != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestThereMustBeAPhilosophersQuestionToAnswerAndAProviderErrorIsReturned(t *testing.T) {
	_, c := client()
	if _, err := Generate(bg, c, nil, nil); !errors.Is(err, ErrNothingToAnswer) {
		t.Fatal(err)
	}
	if _, err := Generate(bg, c, talk[:1], nil); !errors.Is(err, ErrNothingToAnswer) {
		t.Fatal(err)
	}
	_, c = client(llm.Fail(&llm.Error{Kind: llm.KindAuth, Reason: "bad key"}))
	if _, err := Generate(bg, c, talk, nil); !errors.Is(err, llm.ErrAuth) {
		t.Fatalf("%v", err)
	}
	_, c = client(llm.Fail(&llm.Error{Kind: llm.KindOverloaded, Reason: "busy"}), llm.Fail(&llm.Error{Kind: llm.KindOverloaded, Reason: "busy"}), llm.Fail(&llm.Error{Kind: llm.KindOverloaded, Reason: "busy"}))
	if r, err := Generate(bg, c, talk, nil); err == nil || r != nil {
		t.Fatalf("an overloaded provider must not be answered with the general list: %+v %v", r, err)
	}
}
