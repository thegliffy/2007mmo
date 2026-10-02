package protocol

// WornView is the paperdoll layer set that rides with a figure in the
// AOI snapshot. Peers paint from this, so equipping a jerkin changes what
// everyone nearby sees without a second round-trip.
//
// The ids are catalog ids, not sprite keys: the client maps id -> asset
// (paperdoll/jerkin_mask etc.) and tints from the item's WornColor. That
// keeps the sprite pack out of the wire format — a new garment is a
// catalog row plus a mask PNG, no protocol change.
//
// Empty means the slot is bare. Unknown item ids (a client that has not
// seen this catalog revision) paint nothing, which is the same as bare.
type WornView struct {
	// Body is the worn body-layer id (leather jerkin).
	Body string `json:"body,omitempty"`
	// Hand is the wielded item id (sword, axe, knife). Shown on the doll
	// only for items that declare a Worn asset; tools like the pick stay
	// implied by the action animation.
	Hand string `json:"hand,omitempty"`
}

// WornFor projects an equipped map into the view peers paint. Only slots
// whose item declares a Worn asset travel; everything else stays private.
func WornFor(equipped map[string]string) *WornView {
	if len(equipped) == 0 {
		return nil
	}
	cat := Catalog()
	v := WornView{}
	any := false
	for slot, item := range equipped {
		info, ok := cat[item]
		if !ok || info.Worn == "" {
			continue
		}
		switch slot {
		case SlotBody:
			v.Body = item
			any = true
		case SlotHand:
			v.Hand = item
			any = true
		}
	}
	if !any {
		return nil
	}
	return &v
}
