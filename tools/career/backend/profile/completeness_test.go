package profile

import "testing"

func full() completenessInput {
	return completenessInput{
		FirstName:             "Ada",
		LastName:              "Lovelace",
		LinkCount:             1,
		HasImage:              true,
		PersonaCount:          1,
		HasHeadlineAndSummary: true,
		BestSkillCount:        minSkills,
		BestExperienceCount:   1,
	}
}

func TestCompletenessOf_EmptyProfileScoresZero(t *testing.T) {
	got := completenessOf(completenessInput{})
	if got.Score != 0 || got.Band != bandJustStarted {
		t.Fatalf("got score %d band %s, want 0 just-started", got.Score, got.Band)
	}
	if len(got.Items) != 7 {
		t.Fatalf("got %d items, want 7", len(got.Items))
	}
}

func TestCompletenessOf_FullProfileScores100(t *testing.T) {
	got := completenessOf(full())
	if got.Score != 100 || got.Band != bandComplete {
		t.Fatalf("got score %d band %s, want 100 complete", got.Score, got.Band)
	}
}

func TestCompletenessOf_WeightsAddUpTo100(t *testing.T) {
	sum := 0
	for _, it := range completenessOf(completenessInput{}).Items {
		sum += it.Weight
	}
	if sum != 100 {
		t.Fatalf("weights add up to %d, want 100", sum)
	}
}

// Each item on its own, at its boundary: one short of done scores nothing for it, done scores its weight.
func TestCompletenessOf_ItemBoundaries(t *testing.T) {
	cases := []struct {
		name string
		in   completenessInput
		want int
	}{
		{"first name only", completenessInput{FirstName: "Ada"}, 10},
		{"last name only", completenessInput{LastName: "Lovelace"}, 10},
		{"blank name", completenessInput{FirstName: "  ", LastName: "\t"}, 0},
		{"one link", completenessInput{LinkCount: 1}, 25},
		{"two links add nothing", completenessInput{LinkCount: 2}, 25},
		{"image", completenessInput{HasImage: true}, 10},
		{"persona", completenessInput{PersonaCount: 1}, 15},
		{"headline and summary", completenessInput{HasHeadlineAndSummary: true}, 15},
		{"two skills is not enough", completenessInput{BestSkillCount: minSkills - 1}, 0},
		{"three skills", completenessInput{BestSkillCount: minSkills}, 15},
		{"experience", completenessInput{BestExperienceCount: 1}, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := completenessOf(c.in).Score; got != c.want {
				t.Fatalf("score = %d, want %d", got, c.want)
			}
		})
	}
}

func TestCompletenessOf_MissingLinksIsReportedNotDone(t *testing.T) {
	in := full()
	in.LinkCount = 0
	got := completenessOf(in)
	if got.Score != 75 {
		t.Fatalf("score = %d, want 75", got.Score)
	}
	for _, it := range got.Items {
		if it.Key == "links" && it.Done {
			t.Fatal("links item is done with no links")
		}
		if it.Key != "links" && !it.Done {
			t.Fatalf("item %s should be done", it.Key)
		}
	}
}

func TestBandOf_Edges(t *testing.T) {
	cases := map[int]string{
		0: bandJustStarted, 39: bandJustStarted,
		40: bandGettingThere, 69: bandGettingThere,
		70: bandStrong, 99: bandStrong,
		100: bandComplete,
	}
	for score, want := range cases {
		if got := bandOf(score); got != want {
			t.Errorf("bandOf(%d) = %s, want %s", score, got, want)
		}
	}
}
