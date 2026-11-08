package jobmatcher

import "testing"

func TestJobWorkModeIsReadFromTheText(t *testing.T) {
	cases := []struct {
		name string
		job  Job
		want WorkMode
	}{
		// remote
		{"place is just Remote", Job{Title: "Engineer", Location: "Remote"}, WorkRemote},
		{"place Deutschland (Remote)", Job{Title: "Engineer", Location: "Deutschland (Remote)"}, WorkRemote},
		{"place Remote, Germany", Job{Title: "Engineer", Location: "Remote, Germany"}, WorkRemote},
		{"place Homeoffice", Job{Title: "Engineer", Location: "Homeoffice"}, WorkRemote},
		{"place Home Office", Job{Title: "Engineer", Location: "Home Office"}, WorkRemote},
		{"title (Remote)", Job{Title: "Senior Engineer (Remote)", Location: "Berlin"}, WorkRemote},
		{"title ends with - Remote", Job{Title: "Backend Developer - Remote", Location: "Berlin"}, WorkRemote},
		{"title starts with Remote", Job{Title: "Remote Backend Developer", Location: "Berlin"}, WorkRemote},
		{"100 % remote", Job{Title: "Engineer", Location: "Berlin", Description: "Wir bieten dir 100 % Remote Arbeit."}, WorkRemote},
		{"fully remote", Job{Title: "Engineer", Location: "Berlin", Description: "This is a fully remote role."}, WorkRemote},
		{"remote-first", Job{Title: "Engineer", Location: "Berlin", Description: "We are a remote-first company."}, WorkRemote},
		{"vollständig remote", Job{Title: "Engineer", Location: "Berlin", Description: "Die Stelle ist vollständig remote."}, WorkRemote},
		{"komplett remote", Job{Title: "Engineer", Description: "komplett remote möglich: nein, komplett remote."}, WorkRemote},
		{"ortsunabhängig", Job{Title: "Engineer", Location: "Berlin", Description: "Du arbeitest ortsunabhängig."}, WorkRemote},
		{"work from anywhere", Job{Title: "Engineer", Description: "You can work from anywhere in Europe."}, WorkRemote},
		// hybrid
		{"hybrid möglich", Job{Title: "Engineer", Location: "Berlin", Description: "Hybrid möglich."}, WorkHybrid},
		{"hybrid in the title", Job{Title: "Engineer (hybrid)", Location: "Berlin"}, WorkHybrid},
		{"teilweise Home Office", Job{Title: "Engineer", Location: "Berlin", Description: "Teilweise Home Office."}, WorkHybrid},
		{"Home Office möglich on an on-site job", Job{Title: "Erzieher (m/w/d)", Location: "Hamburg", Description: "Wir bieten Fortbildung und Home Office möglich."}, WorkHybrid},
		{"Remote möglich", Job{Title: "Engineer", Location: "Berlin", Description: "Remote möglich."}, WorkHybrid},
		{"mobiles Arbeiten", Job{Title: "Engineer", Location: "Berlin", Description: "Mobiles Arbeiten ist erwünscht."}, WorkHybrid},
		{"two days at home", Job{Title: "Engineer", Location: "Berlin", Description: "2 Tage Home Office pro Woche."}, WorkHybrid},
		{"a city and Remote in the place", Job{Title: "Engineer", Location: "Berlin, Remote"}, WorkHybrid},
		{"a home office perk", Job{Title: "Engineer", Location: "Berlin", Description: "Homeoffice-Zuschuss und Obstkorb."}, WorkHybrid},
		// on-site
		{"a city and a text that says nothing about remote", Job{Title: "Engineer", Location: "Hamburg", Description: officeText}, WorkOnSite},
		{"a postcode and a district", Job{Title: "Erzieher", Location: "20095 Hamburg-Altona", Description: officeText}, WorkOnSite},
		{"several cities", Job{Title: "Engineer", Location: "Düsseldorf, Köln, Essen", Description: officeText}, WorkOnSite},
		// the other senses of "remote" are not a work mode
		{"remote sensing", Job{Title: "Remote Sensing Engineer", Location: "Hamburg", Description: officeText + " Satellite data and remote sensing."}, WorkOnSite},
		{"remote control", Job{Title: "Engineer", Location: "Hamburg", Description: officeText + " Remote control of plants and remote monitoring."}, WorkOnSite},
		// unknown
		{"a place but no description at all", Job{Title: "Engineer", Location: "Hamburg"}, WorkUnknown},
		{"a place and too little text to tell", Job{Title: "Engineer", Location: "Hamburg", Description: "Wir sind ein Team von zehn."}, WorkUnknown},
		{"no place, no mark", Job{Title: "Engineer", Description: officeText}, WorkUnknown},
		{"only a postcode", Job{Title: "Engineer", Location: "20095"}, WorkUnknown},
		{"nothing at all", Job{}, WorkUnknown},
	}
	for _, c := range cases {
		if got := jobWorkMode(c.job); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestOnlyTheStartOfTheTextIsRead(t *testing.T) {
	long := make([]rune, 2100)
	for i := range long {
		long[i] = 'x'
	}
	late := Job{Title: "Engineer", Location: "Berlin", Description: string(long) + " 100 % remote"}
	if got := jobWorkMode(late); got != WorkOnSite {
		t.Errorf("a mark after the first 2,000 characters is not read: %s", got)
	}
	early := Job{Title: "Engineer", Location: "Berlin", Description: "100 % remote " + string(long)}
	if got := jobWorkMode(early); got != WorkRemote {
		t.Errorf("a mark at the start is read: %s", got)
	}
}

func TestRemoteOnlyPlace(t *testing.T) {
	for in, want := range map[string]bool{
		"remote": true, "remote, deutschland": true, "deutschland (remote)": true, "homeoffice": true, "home office": true,
		"remote und europa": true, "remote in eu": true,
		"": false, "hamburg": false, "hamburg, remote": false, "deutschland": false, "berlin (homeoffice)": false,
	} {
		if got := remoteOnlyPlace(in); got != want {
			t.Errorf("remoteOnlyPlace(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestWorkModeNames(t *testing.T) {
	for m, want := range map[WorkMode]string{WorkUnknown: "unknown", WorkOnSite: "on-site", WorkHybrid: "hybrid", WorkRemote: "remote"} {
		if m.String() != want {
			t.Errorf("%d = %q, want %q", m, m.String(), want)
		}
	}
}

func TestAJobIsOnSiteOnlyWithEnoughTextToHaveSaidOtherwise(t *testing.T) {
	at := func(n int) Job {
		return Job{Title: "Engineer", Location: "Hamburg", Description: string(make([]rune, 0)) + repeatRunes('a', n)}
	}
	if got := jobWorkMode(at(WorkModeMinText - 1)); got != WorkUnknown {
		t.Errorf("one character short: %s", got)
	}
	if got := jobWorkMode(at(WorkModeMinText)); got != WorkOnSite {
		t.Errorf("exactly enough: %s", got)
	}
	// characters, not bytes: 150 umlauts are 300 bytes but still 150 characters
	umlauts := Job{Title: "Engineer", Location: "Hamburg", Description: repeatRunes('ä', WorkModeMinText-1)}
	if got := jobWorkMode(umlauts); got != WorkUnknown {
		t.Errorf("one character short, in umlauts: %s", got)
	}
	// a remote mark needs no minimum: a short text can still say it
	short := Job{Title: "Engineer", Location: "Hamburg", Description: "Hybrid möglich."}
	if got := jobWorkMode(short); got != WorkHybrid {
		t.Errorf("a short text that says hybrid: %s", got)
	}
}

func repeatRunes(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
