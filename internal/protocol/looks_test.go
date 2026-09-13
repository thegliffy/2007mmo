package protocol

import (
	"errors"
	"testing"
)

func TestParseLooksAcceptsCatalogIds(t *testing.T) {
	in := Looks{Skin: SkinDeep, Hair: HairLong, HairColor: HairSoot, Top: TopClay}
	got, err := ParseLooks(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != in {
		t.Fatalf("got %+v", got)
	}
}

func TestParseLooksRejectsUnknownAndPartial(t *testing.T) {
	cases := []Looks{
		{},
		{Skin: SkinTan},
		{Skin: "peach", Hair: HairShort, HairColor: HairUmber, Top: TopMoss},
		{Skin: SkinTan, Hair: "mullet", HairColor: HairUmber, Top: TopMoss},
	}
	for _, in := range cases {
		_, err := ParseLooks(in)
		if !errors.Is(err, ErrBadLooks) {
			t.Fatalf("ParseLooks(%+v) = %v, want ErrBadLooks", in, err)
		}
	}
}

func TestLooksSetRequiresEveryAxis(t *testing.T) {
	if (Looks{}).Set() {
		t.Fatal("empty looks must not count as finished")
	}
	full := Looks{Skin: SkinTan, Hair: HairShort, HairColor: HairUmber, Top: TopMoss}
	if !full.Set() {
		t.Fatal("a complete face must count as finished")
	}
}
