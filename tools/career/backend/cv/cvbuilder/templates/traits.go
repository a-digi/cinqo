package templates

// The order of the designs in the template picker (plan 254 and 255): the designs with two columns and a photo come first, the ATS-friendly designs (meant to have no
// photo) last, every other design between them. The registry (metas) stays the declarative list of designs and the order inside each group; this file only says which
// designs belong to the first group.

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

// atsCategory is the category of the designs that are meant to have no photo: the ats-* designs are deliberately plain, one-column and photo-free layouts so that an ATS
// reads their PDF cleanly (see the registry's own comment). They are the one-column designs "without a photo" and come last in the picker (plan 255).
const atsCategory = "ATS-Friendly"

// pickerOrder is metas in three groups: the two-column photo designs, then every other one-column design (they are built to show a photo), then the ATS-friendly
// designs. Each group keeps the registry order, so nothing else moves. Made once: the list never changes.
var pickerOrder = partitionForPicker(metas, twoColumnPhotoDesigns)

// pickerRank is the group of a design: 0 the two-column photo designs, 2 the ATS-friendly ones, 1 every other design. A design that is in both (two columns and
// ATS-friendly) is in the first group.
func pickerRank(m TemplateMeta, twoColumn map[string]bool) int {
	switch {
	case twoColumn[m.ID]:
		return 0
	case m.Category == atsCategory:
		return 2
	default:
		return 1
	}
}

// partitionForPicker returns a new slice with the designs of rank 0, then rank 1, then rank 2, each group in the order of all. Nothing is dropped or doubled.
func partitionForPicker(all []TemplateMeta, twoColumn map[string]bool) []TemplateMeta {
	out := make([]TemplateMeta, 0, len(all))
	for rank := 0; rank <= 2; rank++ {
		for _, m := range all {
			if pickerRank(m, twoColumn) == rank {
				out = append(out, m)
			}
		}
	}
	return out
}
