package protocol

import "fmt"

// Appearance axes for the v0 creator. Four knobs, no stats, no class.
// Palette names are Hollowmere woodland — original, not a studio clone.
const (
	SkinFair  = "fair"
	SkinTan   = "tan"
	SkinOlive = "olive"
	SkinDeep  = "deep"

	HairCropped = "cropped"
	HairShort   = "short"
	HairTied    = "tied"
	HairLong    = "long"

	HairUmber  = "umber"
	HairStraw  = "straw"
	HairSoot   = "soot"
	HairRusset = "russet"
	HairSnow   = "snow"

	TopMoss  = "moss"
	TopClay  = "clay"
	TopInk   = "ink"
	TopCream = "cream"
	TopBerry = "berry"
)

// Looks is what the hamlet can see of a person: skin, hair, and tunic.
// Empty (zero) means the creator has not been finished.
type Looks struct {
	Skin      string `json:"skin,omitempty"`
	Hair      string `json:"hair,omitempty"`
	HairColor string `json:"hairColor,omitempty"`
	Top       string `json:"top,omitempty"`
}

// LooksOption is one swatch or style the creator can pick.
type LooksOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// LooksCatalog is the closed list of valid axes. The client renders from
// this rather than a list compiled into the page.
type LooksCatalog struct {
	Skin      []LooksOption `json:"skin"`
	Hair      []LooksOption `json:"hair"`
	HairColor []LooksOption `json:"hairColor"`
	Top       []LooksOption `json:"top"`
}

// LooksInfo is GET /auth/looks — current row plus the picker catalog.
type LooksInfo struct {
	Username   string       `json:"username"`
	World      string       `json:"world"`
	Looks      *Looks       `json:"looks,omitempty"`
	NeedsLooks bool         `json:"needsLooks,omitempty"`
	Catalog    LooksCatalog `json:"catalog"`
}

// Set reports whether every axis is present. Used as "has finished the
// creator" — a partial row is treated as unfinished.
func (l Looks) Set() bool {
	return l.Skin != "" && l.Hair != "" && l.HairColor != "" && l.Top != ""
}

// ParseLooks accepts only catalog ids. Unknown or partial input is refused
// rather than coerced, so a spam-click cannot mint a surprise palette.
func ParseLooks(in Looks) (Looks, error) {
	cat := AppearanceCatalog()
	out := Looks{
		Skin:      pickOption(cat.Skin, in.Skin),
		Hair:      pickOption(cat.Hair, in.Hair),
		HairColor: pickOption(cat.HairColor, in.HairColor),
		Top:       pickOption(cat.Top, in.Top),
	}
	if !out.Set() {
		return Looks{}, fmt.Errorf("%w: pick a skin, a hair, and a tunic", ErrBadLooks)
	}
	return out, nil
}

// ErrBadLooks is a client error: the posted ids are not in the catalog.
var ErrBadLooks = errBadLooks{}

type errBadLooks struct{}

func (errBadLooks) Error() string { return "those looks are not ones the hamlet knows" }

func pickOption(opts []LooksOption, id string) string {
	for _, o := range opts {
		if o.ID == id {
			return o.ID
		}
	}
	return ""
}

// AppearanceCatalog is the closed v0 palette. Hex colors travel with it
// so the paperdoll and the creator swatches stay in agreement.
func AppearanceCatalog() LooksCatalog {
	return LooksCatalog{
		Skin: []LooksOption{
			{ID: SkinFair, Name: "Fair", Color: "#f0d2b0"},
			{ID: SkinTan, Name: "Tan", Color: "#d4a574"},
			{ID: SkinOlive, Name: "Olive", Color: "#b08a58"},
			{ID: SkinDeep, Name: "Deep", Color: "#6b4226"},
		},
		Hair: []LooksOption{
			{ID: HairCropped, Name: "Cropped"},
			{ID: HairShort, Name: "Short"},
			{ID: HairTied, Name: "Tied"},
			{ID: HairLong, Name: "Long"},
		},
		HairColor: []LooksOption{
			{ID: HairUmber, Name: "Umber", Color: "#3d2412"},
			{ID: HairStraw, Name: "Straw", Color: "#d4b46a"},
			{ID: HairSoot, Name: "Soot", Color: "#1c140c"},
			{ID: HairRusset, Name: "Russet", Color: "#8a3a1c"},
			{ID: HairSnow, Name: "Snow", Color: "#e8e0d0"},
		},
		Top: []LooksOption{
			{ID: TopMoss, Name: "Moss", Color: "#4a6a32"},
			{ID: TopClay, Name: "Clay", Color: "#a85a32"},
			{ID: TopInk, Name: "Ink", Color: "#2a3a5a"},
			{ID: TopCream, Name: "Cream", Color: "#e8d4a8"},
			{ID: TopBerry, Name: "Berry", Color: "#7a2a40"},
		},
	}
}

// DefaultLooks is what a figure wears when the row has no creator pass
// (load bots, leftover rows). Tan, short umber, moss tunic.
func DefaultLooks() Looks {
	return Looks{Skin: SkinTan, Hair: HairShort, HairColor: HairUmber, Top: TopMoss}
}
