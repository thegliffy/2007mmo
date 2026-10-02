package protocol

import "testing"

// The doll only shows what the catalog says shows. A pick in hand is
// equipment, but it is not a garment — it should not paint.
func TestWornForOnlyShipsPaintedGear(t *testing.T) {
	if v := WornFor(nil); v != nil {
		t.Fatal("no gear should be a nil view, not an empty one")
	}
	v := WornFor(map[string]string{SlotHand: ItemPick})
	if v != nil {
		t.Fatalf("the pick has no Worn asset; got %+v", v)
	}
	v = WornFor(map[string]string{SlotBody: ItemJerkin, SlotHand: ItemSword})
	if v == nil || v.Body != ItemJerkin || v.Hand != ItemSword {
		t.Fatalf("jerkin and sword should both paint: %+v", v)
	}
	// An id the catalog does not know paints nothing rather than panicking
	// or shipping a bogus key.
	v = WornFor(map[string]string{SlotBody: "not-a-real-item"})
	if v != nil {
		t.Fatalf("unknown id should not ship: %+v", v)
	}
}

// Every Worn asset needs a tint, or the client paints a transparent
// garment and wonders why.
func TestWornAssetsCarryTheirColor(t *testing.T) {
	for id, info := range Catalog() {
		if info.Worn != "" && info.WornColor == "" {
			t.Errorf("%s declares Worn=%q with no WornColor", id, info.Worn)
		}
	}
}
