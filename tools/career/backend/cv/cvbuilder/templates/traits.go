package templates

// The order of the designs in the template picker (plan 254): the designs with two columns and a photo come first, every other design after them. The registry (metas)
// stays the declarative list of designs and the order inside each group; this file only says which designs belong to the first group.

// twoColumnPhotoDesigns are the designs whose page has a second column (a sidebar, or the aside of executive-charcoal) and a photo as part of the layout. The list was
// made by looking at a rendering of every design with a photo attached (plan 254, step 1): the 20 designs whose markup has a `class="sidebar"` (the photo opens the
// sidebar) and executive-charcoal (two columns under a header that holds the photo). A design is not listed because its entries use a grid for the dates, or because
// it has a coloured header band: that is one column. A design added later is after them until someone adds it here.
var twoColumnPhotoDesigns = map[string]bool{
	"bold-blue-sidebar":         true,
	"clean-blue-sidebar":        true,
	"contemporary-navy-sidebar": true,
	"corporate-gold-sidebar":    true,
	"executive-charcoal":        true,
	"friendly-orange-sidebar":   true,
	"green-gradient-sidebar":    true,
	"grey-pro-sidebar":          true,
	"leadership-brown-sidebar":  true,
	"maker-dark-sidebar":        true,
	"midnight-gold-sidebar":     true,
	"modern-sidebar":            true,
	"refined-red-sidebar":       true,
	"side-coral":                true,
	"side-graphite-right":       true,
	"side-navy":                 true,
	"side-night-mono":           true,
	"side-sage":                 true,
	"side-sand-serif":           true,
	"side-slate-wide":           true,
	"side-teal-banner":          true,
}

// pickerOrder is metas with the two-column photo designs first. Each group keeps the registry order, so nothing else moves. Made once: the list never changes.
var pickerOrder = partitionTwoColumnFirst(metas, twoColumnPhotoDesigns)

// partitionTwoColumnFirst returns a new slice with the designs named in first before the others, each group in the order of all. Nothing is dropped or doubled.
func partitionTwoColumnFirst(all []TemplateMeta, first map[string]bool) []TemplateMeta {
	out := make([]TemplateMeta, 0, len(all))
	for _, m := range all {
		if first[m.ID] {
			out = append(out, m)
		}
	}
	for _, m := range all {
		if !first[m.ID] {
			out = append(out, m)
		}
	}
	return out
}
