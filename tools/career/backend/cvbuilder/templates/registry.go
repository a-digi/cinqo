package templates

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

// TemplateMeta describes one selectable design — returned as-is by
// ListTemplates for the frontend's own template picker.
//
// Category is one fixed style bucket (Classic/Modern/ATS-Friendly/
// Creative/Executive/Minimalist) — the frontend's own single-select
// style filter. BestFor is a list of profession tags drawn from a
// small SHARED vocabulary (see professionTags below) — the frontend's
// own multi-select profession filter groups by exact string match, so
// every entry below reuses those exact strings rather than inventing
// its own free-text variant per template.
type TemplateMeta struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	BestFor     []string `json:"bestFor"`
}

// professionTags is the fixed, shared vocabulary every metas[].BestFor
// entry below draws from — kept as one list here (not enforced at
// compile time, just documented) so the frontend's own profession
// filter groups templates correctly instead of silently missing one
// because of a typo'd near-duplicate tag.
//
//	Software & IT, Marketing & Sales, Finance & Accounting, Healthcare,
//	Design & Creative, Executive & Management, Education & Academia,
//	Customer Service, Legal, Engineering, Entry-Level & Internships,
//	Human Resources, Project Management
//
// The 15 ats-* designs are all deliberately single-column, plain-text
// layouts — no tables, no CSS `columns`, no floats, no absolute/fixed
// positioning, no profile photo — since any of those can reorder or
// drop content in an ATS's own PDF text-layer extraction relative to
// what's visually shown. They differ from each other only in
// typography, accent color, and (for ats-skills-first) section order.
// The 8 templates after them (modern-banner onward) trade that
// constraint for real visual flair — color blocks, accent rules, a
// timeline marker — since they're explicitly categorized Modern/
// Creative/Executive/Minimalist, not ATS-Friendly; they still stay
// single-column/normal-flow throughout (no position:fixed, no tables),
// since modern-sidebar already proved that kind of layout risks real
// pagination bugs for a purely decorative gain. See
// plan/ai/career/cv-builder/step-05-ats-templates.md and
// plan/ai/career/cv-builder/step-06-template-gallery-and-filters.md.
var metas = []TemplateMeta{
	{ID: "modern-mono", Name: "Modern Mono", Description: "Single accent color, minimal, one column.",
		Category: "Modern", BestFor: []string{"Software & IT", "Marketing & Sales", "Design & Creative"}},
	{ID: "modern-sidebar", Name: "Modern Sidebar", Description: "Two-column layout with a colored sidebar for contact info and skills.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Design & Creative", "Project Management"}},
	{ID: "classic", Name: "Classic", Description: "Traditional serif layout, closest to a conventional printed resume.",
		Category: "Classic", BestFor: []string{"Finance & Accounting", "Legal", "Education & Academia"}},
	{ID: "ats-clean", Name: "ATS Clean", Description: "Single-column sans-serif with minimal navy accents — built for maximum ATS parsing reliability.",
		Category: "ATS-Friendly", BestFor: []string{"Software & IT", "Engineering", "Customer Service"}},
	{ID: "ats-serif-classic", Name: "ATS Serif Classic", Description: "Traditional serif resume in black text only, no color at all — the most conservative ATS-safe option.",
		Category: "ATS-Friendly", BestFor: []string{"Legal", "Finance & Accounting", "Education & Academia"}},
	{ID: "ats-minimal-gray", Name: "ATS Minimal Gray", Description: "Uppercase section headers, generous whitespace, no rules or color accents anywhere.",
		Category: "ATS-Friendly", BestFor: []string{"Human Resources", "Customer Service", "Entry-Level & Internships"}},
	{ID: "ats-executive", Name: "ATS Executive", Description: "Serif headings with a single rule under the name, charcoal accents, formal tone.",
		Category: "ATS-Friendly", BestFor: []string{"Executive & Management", "Finance & Accounting"}},
	{ID: "ats-compact", Name: "ATS Compact", Description: "Smaller type and tighter spacing to fit more content on a single page.",
		Category: "ATS-Friendly", BestFor: []string{"Engineering", "Software & IT"}},
	{ID: "ats-modern-sans", Name: "ATS Modern Sans", Description: "Clean sans-serif body with thin teal underlines on section headings.",
		Category: "ATS-Friendly", BestFor: []string{"Software & IT", "Project Management"}},
	{ID: "ats-times", Name: "ATS Times", Description: "Pure Times New Roman with maroon section headings — the most traditional printed-resume look.",
		Category: "ATS-Friendly", BestFor: []string{"Legal", "Education & Academia"}},
	{ID: "ats-verdana", Name: "ATS Verdana", Description: "Verdana-based body text, slate-blue accents, wide letter-spacing on headings.",
		Category: "ATS-Friendly", BestFor: []string{"Human Resources", "Customer Service"}},
	{ID: "ats-tahoma", Name: "ATS Tahoma", Description: "Tahoma-based body text, burgundy accents, compact single-line contact header.",
		Category: "ATS-Friendly", BestFor: []string{"Finance & Accounting", "Project Management"}},
	{ID: "ats-calibri", Name: "ATS Calibri Style", Description: "Calibri-style sans-serif with warm brown accents and a soft, rounded feel.",
		Category: "ATS-Friendly", BestFor: []string{"Marketing & Sales", "Human Resources"}},
	{ID: "ats-garamond", Name: "ATS Garamond Style", Description: "Garamond-style serif with deep green accents — understated and elegant.",
		Category: "ATS-Friendly", BestFor: []string{"Education & Academia", "Legal"}},
	{ID: "ats-bold-headers", Name: "ATS Bold Headers", Description: "Grayscale only — bold uppercase headings with a bottom rule, zero color risk.",
		Category: "ATS-Friendly", BestFor: []string{"Engineering", "Software & IT"}},
	{ID: "ats-two-line-header", Name: "ATS Two-Line Header", Description: "Name and contact details centered on their own lines, gray accents.",
		Category: "ATS-Friendly", BestFor: []string{"Customer Service", "Entry-Level & Internships"}},
	{ID: "ats-skills-first", Name: "ATS Skills First", Description: "Leads with Skills before Experience — suited to skill-heavy or career-change resumes.",
		Category: "ATS-Friendly", BestFor: []string{"Software & IT", "Entry-Level & Internships", "Engineering"}},
	{ID: "ats-academic", Name: "ATS Academic", Description: "Centered header, small-caps-style section titles, black-only styling for academic or formal CVs.",
		Category: "ATS-Friendly", BestFor: []string{"Education & Academia"}},
	{ID: "modern-banner", Name: "Modern Banner", Description: "A bold colored header banner behind your name and contact details.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Design & Creative"}},
	{ID: "modern-timeline", Name: "Modern Timeline", Description: "Experience entries connected by a vertical timeline line and markers.",
		Category: "Modern", BestFor: []string{"Software & IT", "Project Management", "Engineering"}},
	{ID: "creative-accent", Name: "Creative Accent", Description: "Playful purple accents, italic headline, rounded skill pills.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "creative-bold", Name: "Creative Bold", Description: "Oversized name, a bold color block behind your headline, bold section rules.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "executive-elegant", Name: "Executive Elegant", Description: "Centered, refined serif layout with a rule above and below the contact line.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting", "Legal"}},
	{ID: "executive-serif-bold", Name: "Executive Serif Bold", Description: "Bold serif headings with a double rule under your name — formal and assertive.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Legal"}},
	{ID: "minimalist-lines", Name: "Minimalist Lines", Description: "Hairline rules, light section labels, generous whitespace throughout.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Education & Academia", "Engineering"}},
	{ID: "minimalist-airy", Name: "Minimalist Airy", Description: "No rules at all — just spacing and weight contrast carrying the whole design.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Software & IT"}},
	{ID: "modern-gradient", Name: "Modern Aurora", Description: "Indigo-to-violet gradient header card with a ringed round photo and soft skill chips.",
		Category: "Modern", BestFor: []string{"Software & IT", "Marketing & Sales"}},
	{ID: "modern-split-header", Name: "Modern Ledger", Description: "Bold name with a framed photo, a ruled contact strip and numbered sections.",
		Category: "Modern", BestFor: []string{"Project Management", "Engineering"}},
	{ID: "modern-card", Name: "Modern Cards", Description: "Soft gray page with white section cards, an orange accent and a ringed round photo.",
		Category: "Modern", BestFor: []string{"Software & IT", "Design & Creative"}},
	{ID: "modern-dark-header", Name: "Modern Noir", Description: "Dark rounded header with teal accents, an outlined photo and uppercase skill tags.",
		Category: "Modern", BestFor: []string{"Software & IT", "Engineering"}},
	{ID: "modern-dots", Name: "Modern Mint", Description: "Fresh mint timeline with dot markers, a round photo and a three-column skills list.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Entry-Level & Internships"}},
	{ID: "modern-photo-hero", Name: "Modern Portrait", Description: "Large portrait photo beside your name and summary, with blue section markers.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Human Resources"}},
	{ID: "creative-pastel", Name: "Creative Peach", Description: "Warm cream header, arch-shaped portrait, serif italic headings and soft chips.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "creative-monogram", Name: "Creative Editorial", Description: "Magazine-style oversized name, red italic numerals and a bold top rule.",
		Category: "Creative", BestFor: []string{"Design & Creative"}},
	{ID: "creative-duotone", Name: "Creative Duotone", Description: "Deep purple header with a full-height photo panel and gold accents.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "creative-stripe", Name: "Creative Geometric", Description: "Teal side bar, double-ringed round photo and amber date tags.",
		Category: "Creative", BestFor: []string{"Design & Creative"}},
	{ID: "creative-sunset", Name: "Creative Coral", Description: "Coral gradient band with an overlapping centered photo and solid coral skill tags.",
		Category: "Creative", BestFor: []string{"Marketing & Sales", "Design & Creative"}},
	{ID: "executive-navy", Name: "Executive Navy", Description: "Navy header with a gold rule, spaced serif capitals and a gold-framed portrait.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting"}},
	{ID: "executive-monogram-rule", Name: "Executive Centered", Description: "Centered serif header with a ringed round photo, double rule and flanked section titles.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Legal"}},
	{ID: "executive-charcoal", Name: "Executive Slate", Description: "Small-caps name, framed photo and a gray details panel beside the summary.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting"}},
	{ID: "minimalist-photo-left", Name: "Minimalist Quiet", Description: "Light-weight name, small round photo and hairline section labels in a left column.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Software & IT"}},
	{ID: "minimalist-whitespace", Name: "Minimalist Air", Description: "Centered, ultra-light header with generous whitespace and no rules.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Education & Academia"}},
	{ID: "minimalist-mono-caps", Name: "Minimalist Mono", Description: "Monospace spec-sheet look with boxed sections, tab labels and a grayscale photo.",
		Category: "Minimalist", BestFor: []string{"Software & IT", "Engineering"}},
	{ID: "classic-portrait", Name: "Classic Ink", Description: "Palatino serif, framed portrait, double rule and small-caps headings in black ink.",
		Category: "Classic", BestFor: []string{"Finance & Accounting", "Education & Academia"}},
	{ID: "classic-ribbon", Name: "Classic Burgundy", Description: "Centered burgundy name, ringed round photo, small-caps headings and italic dates.",
		Category: "Classic", BestFor: []string{"Legal", "Human Resources"}},
	{ID: "classic-centered-photo", Name: "Classic Sage", Description: "Oval portrait above a sage-green serif name, with short-rule section titles.",
		Category: "Classic", BestFor: []string{"Education & Academia", "Customer Service"}},
	{ID: "modern-slate-line", Name: "Modern Slate Line", Description: "Quiet slate palette, left photo and ruled section titles with a clean date column.",
		Category: "Modern", BestFor: []string{"Software & IT", "Engineering"}},
	{ID: "modern-indigo-strip", Name: "Modern Indigo Strip", Description: "Large name, ruled contact strip and numbered sections in indigo.",
		Category: "Modern", BestFor: []string{"Software & IT", "Project Management"}},
	{ID: "modern-ocean-banner", Name: "Modern Ocean Banner", Description: "Cyan banner header with a white-ringed photo and solid skill tags.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Customer Service"}},
	{ID: "modern-berry-tint", Name: "Modern Berry Tint", Description: "Soft berry card header with a tall photo and pill section labels.",
		Category: "Modern", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "modern-emerald-timeline", Name: "Modern Emerald Timeline", Description: "Emerald timeline with dot markers and a three-column skills grid.",
		Category: "Modern", BestFor: []string{"Software & IT", "Project Management", "Engineering"}},
	{ID: "modern-copper-right", Name: "Modern Copper Right", Description: "Copper accents, photo on the right and shaded section bars.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Human Resources"}},
	{ID: "modern-sky-hero", Name: "Modern Sky Hero", Description: "Tall portrait beside your name and summary, sky-blue underlined headings.",
		Category: "Modern", BestFor: []string{"Software & IT", "Human Resources"}},
	{ID: "modern-graphite-frame", Name: "Modern Graphite Frame", Description: "Framed header box, graphite palette and outlined skills.",
		Category: "Modern", BestFor: []string{"Engineering", "Project Management"}},
	{ID: "creative-plum-duo", Name: "Creative Plum Duo", Description: "Plum header with a full-height photo panel and diamond section markers.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "creative-amber-overlap", Name: "Creative Amber Overlap", Description: "Warm gradient band with an overlapping centered photo.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "creative-rose-rule", Name: "Creative Rose Rule", Description: "Oversized uppercase name under a heavy rule, rose numerals and slashes.",
		Category: "Creative", BestFor: []string{"Design & Creative"}},
	{ID: "creative-teal-tint", Name: "Creative Teal Tint", Description: "Teal tinted header card, serif italic headings and rounded chips.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Education & Academia"}},
	{ID: "creative-olive-banner", Name: "Creative Olive Banner", Description: "Olive banner header with a timeline and uppercase tag skills.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Education & Academia"}},
	{ID: "creative-violet-center", Name: "Creative Violet Center", Description: "Centered violet header with a ringed photo and flanked section titles.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "executive-crimson-center", Name: "Executive Crimson Center", Description: "Centered serif header in crimson with flanked section titles.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Legal"}},
	{ID: "executive-navy-rule", Name: "Executive Navy Rule", Description: "Heavy top rule, uppercase serif name and small-caps headings in navy.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting"}},
	{ID: "executive-gold-dark", Name: "Executive Gold Dark", Description: "Dark header with a gold rule and a framed photo, Georgia serif body.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting"}},
	{ID: "executive-forest-frame", Name: "Executive Forest Frame", Description: "Framed header box in forest green with a formal Palatino layout.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Education & Academia"}},
	{ID: "executive-steel-strip", Name: "Executive Steel Strip", Description: "Steel-blue strip header, ruled sections and uppercase skill tags.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Project Management"}},
	{ID: "executive-charcoal-right", Name: "Executive Charcoal Right", Description: "Charcoal palette with the portrait on the right and bar headings.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Legal"}},
	{ID: "minimalist-plain-left", Name: "Minimalist Plain Left", Description: "Black and white, left photo, tiny gray labels and nothing extra.",
		Category: "Minimalist", BestFor: []string{"Software & IT", "Design & Creative"}},
	{ID: "minimalist-plain-center", Name: "Minimalist Plain Center", Description: "Centered header, soft slate tone and stacked entries.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Education & Academia"}},
	{ID: "minimalist-line-right", Name: "Minimalist Line Right", Description: "Charcoal with a right-hand photo and hairline section rules.",
		Category: "Minimalist", BestFor: []string{"Engineering", "Software & IT"}},
	{ID: "minimalist-sage-rule", Name: "Minimalist Sage Rule", Description: "Heavy top rule, sage-green accent and a quiet skills grid.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Education & Academia"}},
	{ID: "minimalist-steel-strip", Name: "Minimalist Steel Strip", Description: "Steel-blue strip header with ruled rows and no ornament.",
		Category: "Minimalist", BestFor: []string{"Software & IT", "Engineering"}},
	{ID: "minimalist-sand-frame", Name: "Minimalist Sand Frame", Description: "Thin framed header, warm copper tone and stacked entries.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Human Resources"}},
	{ID: "classic-navy-right", Name: "Classic Navy Right", Description: "Palatino serif in navy, photo on the right and small-caps headings.",
		Category: "Classic", BestFor: []string{"Finance & Accounting", "Legal"}},
	{ID: "classic-berry-center", Name: "Classic Berry Center", Description: "Centered Palatino header in berry with a date column.",
		Category: "Classic", BestFor: []string{"Education & Academia", "Human Resources"}},
	{ID: "classic-ink-left", Name: "Classic Ink Left", Description: "Pure black ink, left photo and small-caps rules.",
		Category: "Classic", BestFor: []string{"Legal", "Finance & Accounting"}},
	{ID: "classic-forest-strip", Name: "Classic Forest Strip", Description: "Georgia serif in forest green with a ruled contact strip.",
		Category: "Classic", BestFor: []string{"Education & Academia", "Legal"}},
	{ID: "classic-gold-tint", Name: "Classic Gold Tint", Description: "Soft gold header card with arch portrait and italic headings.",
		Category: "Classic", BestFor: []string{"Education & Academia", "Customer Service"}},
	{ID: "classic-crimson-rule", Name: "Classic Crimson Rule", Description: "Heavy rule and uppercase serif name with crimson small-caps headings.",
		Category: "Classic", BestFor: []string{"Legal", "Finance & Accounting"}},
	{ID: "side-navy", Name: "Sidebar Navy", Description: "Navy left sidebar with a round photo, contact and skills; summary and experience on the right.",
		Category: "Modern", BestFor: []string{"Software & IT", "Project Management", "Engineering"}},
	{ID: "side-sage", Name: "Sidebar Sage", Description: "Light sage left sidebar with dark text, a rounded photo and a calm green accent.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Education & Academia"}},
	{ID: "side-graphite-right", Name: "Sidebar Graphite Right", Description: "Graphite sidebar on the right with a framed photo, serif headings in the main column.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting"}},
	{ID: "side-coral", Name: "Sidebar Coral", Description: "Bold coral left sidebar with a square photo and white chips.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "side-slate-wide", Name: "Sidebar Slate Wide", Description: "Wide slate sidebar with the name inside it, chip skills and a clean main column.",
		Category: "Modern", BestFor: []string{"Software & IT", "Customer Service"}},
	{ID: "side-sand-serif", Name: "Sidebar Sand Serif", Description: "Warm sand sidebar, Palatino serif and a burgundy accent.",
		Category: "Classic", BestFor: []string{"Legal", "Education & Academia", "Human Resources"}},
	{ID: "side-teal-banner", Name: "Sidebar Teal", Description: "Deep teal sidebar with a mint accent and pill skills.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Software & IT"}},
	{ID: "side-night-mono", Name: "Sidebar Night Mono", Description: "Near-black sidebar with monospace labels, a grayscale photo and a lime accent.",
		Category: "Minimalist", BestFor: []string{"Software & IT", "Engineering"}},
	{ID: "clean-blue-sidebar", Name: "Clean Blue Sidebar", Description: "White page with a pale, subtle sidebar, thin divider and a single blue accent.",
		Category: "Minimalist", BestFor: []string{"Software & IT", "Customer Service", "Human Resources"}},
	{ID: "corporate-gold-sidebar", Name: "Corporate Gold Sidebar", Description: "Charcoal sidebar with fine gold rules and a framed portrait; formal serif main column.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting", "Legal"}},
	{ID: "contemporary-navy-sidebar", Name: "Contemporary Navy Sidebar", Description: "Deep navy sidebar with soft pill skills and understated accents.",
		Category: "Modern", BestFor: []string{"Software & IT", "Project Management", "Marketing & Sales"}},
	{ID: "maker-dark-sidebar", Name: "Maker Dark Sidebar", Description: "Near-black sidebar with a hexagon photo, glowing yellow and pink shapes and the name set in the sidebar.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "refined-red-sidebar", Name: "Refined Red Sidebar", Description: "Airy off-white sidebar closed by a slim red edge, with red section titles.",
		Category: "Classic", BestFor: []string{"Legal", "Finance & Accounting", "Human Resources"}},
	{ID: "leadership-brown-sidebar", Name: "Leadership Brown Sidebar", Description: "Warm light-brown sidebar with a fine woven pattern and plum accents; formal and calm.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Education & Academia"}},
	{ID: "green-gradient-sidebar", Name: "Green Gradient Sidebar", Description: "Sleek sidebar in a deep-to-fresh green gradient with pale pills.",
		Category: "Modern", BestFor: []string{"Software & IT", "Marketing & Sales"}},
	{ID: "bold-blue-sidebar", Name: "Bold Blue Sidebar", Description: "Strong blue sidebar with big orange and red circles, a white-ringed photo and warm section marks.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "midnight-gold-sidebar", Name: "Midnight Gold Sidebar", Description: "Dark sidebar topped with a thick gold bar, gold section labels and a round photo.",
		Category: "Modern", BestFor: []string{"Executive & Management", "Project Management"}},
	{ID: "grey-pro-sidebar", Name: "Grey Pro Sidebar", Description: "Professional light-grey sidebar with dark text, a rounded photo and ruled skills.",
		Category: "Modern", BestFor: []string{"Engineering", "Customer Service", "Finance & Accounting"}},
	{ID: "friendly-orange-sidebar", Name: "Friendly Orange Sidebar", Description: "Orange sidebar with a warm rounded intro card around your name in the main column.",
		Category: "Creative", BestFor: []string{"Customer Service", "Marketing & Sales", "Human Resources"}},
	{ID: "photo-panel-steel", Name: "Photo Panel Steel", Description: "Large portrait panel on the left of a soft steel-blue header card, contact stacked beside it.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Human Resources", "Customer Service"}},
	{ID: "paint-splash", Name: "Paint Splash", Description: "Playful header over pastel paint-splash shapes drawn in the template itself, round photo and bright chips.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "square-teal", Name: "Square Teal", Description: "Professional layout built from squares: square photo, block heading markers, square tags and a tiled corner ornament.",
		Category: "Modern", BestFor: []string{"Engineering", "Project Management", "Finance & Accounting"}},
	{ID: "orange-frame", Name: "Orange Frame", Description: "Whole CV set inside a slim orange border that repeats on every page, with an orange-ruled header.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Customer Service", "Entry-Level & Internships"}},
	{ID: "dotted-lime", Name: "Dotted Lime", Description: "Minimalist page drawn with dotted rules in lime green; dark green text keeps contrast.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Entry-Level & Internships", "Education & Academia"}},
	{ID: "night-mode", Name: "Night Mode", Description: "Full dark page with light text and a sky-blue accent that fills every page to the edge.",
		Category: "Modern", BestFor: []string{"Software & IT", "Engineering", "Design & Creative"}},
	{ID: "gilded-frame", Name: "Gilded Frame", Description: "Clean white page inside a fine double gold frame, centered spaced-capitals name and gold diamond skills.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Legal", "Education & Academia"}},
	{ID: "green-colorful", Name: "Green Colorful", Description: "Simple, friendly layout where each section gets its own green and skills cycle through fresh greens.",
		Category: "Creative", BestFor: []string{"Customer Service", "Entry-Level & Internships", "Education & Academia"}},
	{ID: "classic-red-columns", Name: "Classic Red Columns", Description: "Classic serif page: framed photo, red name, and the contact details in their own ruled column.",
		Category: "Classic", BestFor: []string{"Legal", "Finance & Accounting", "Education & Academia"}},
	{ID: "rich-gold-cream", Name: "Rich Gold Cream", Description: "Cream page with a deep espresso header, gold gradient rule, oval portrait and gold diamond skills.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting", "Legal"}},
	{ID: "retro-green-double", Name: "Retro Green Double", Description: "Retro typewriter look on cream paper: double-ruled green name box, sepia photo and reversed green section labels.",
		Category: "Classic", BestFor: []string{"Design & Creative", "Education & Academia", "Entry-Level & Internships"}},
	{ID: "gradient-blue-diagonal", Name: "Gradient Blue Diagonal", Description: "Blue-to-cyan gradient header cut on a diagonal, ringed photo, gradient rules and gradient skill chips.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales", "Software & IT"}},
	{ID: "skills-first-orange", Name: "Skills First Orange", Description: "Clean skill-based CV: your skills sit in a highlighted orange panel directly under the header, before the experience.",
		Category: "Modern", BestFor: []string{"Entry-Level & Internships", "Customer Service", "Marketing & Sales"}},
}

// ListTemplates returns every available design, in a stable, fixed
// order — a plain Go function, not its own HTTP route; step 3's own
// GET .../cv-documents/templates handler just calls this and returns
// the result as JSON.
func ListTemplates() []TemplateMeta {
	return metas
}

// Each id below is both a metas[].ID value AND a real subdirectory
// here (modern-mono/, modern-sidebar/, classic/) — go:embed can't use
// a variable/wildcard tied to that Go slice, so this list is
// necessarily its own small, parallel source of truth; adding a
// template means updating both this directive and metas above (a
// mismatch is caught immediately: Render's own isKnownTemplate check
// or ParseFS's own file-not-found error, never a silent bad render).
// See this directory's own sync-from-frontend.sh — the actual
// template.html/style.css files embedded here are COPIES; the real
// source of truth is
// tools/career/frontend-app/src/Components/CvBuilder/Template/<id>/,
// synced into these folders by that script before every `go build`
// (Go's own //go:embed can only reach files inside this package's own
// directory tree, never across into frontend-app/).
//
//go:embed modern-mono/template.html modern-mono/style.css modern-sidebar/template.html modern-sidebar/style.css classic/template.html classic/style.css
//go:embed ats-clean/template.html ats-clean/style.css ats-serif-classic/template.html ats-serif-classic/style.css
//go:embed ats-minimal-gray/template.html ats-minimal-gray/style.css ats-executive/template.html ats-executive/style.css
//go:embed ats-compact/template.html ats-compact/style.css ats-modern-sans/template.html ats-modern-sans/style.css
//go:embed ats-times/template.html ats-times/style.css ats-verdana/template.html ats-verdana/style.css
//go:embed ats-tahoma/template.html ats-tahoma/style.css ats-calibri/template.html ats-calibri/style.css
//go:embed ats-garamond/template.html ats-garamond/style.css ats-bold-headers/template.html ats-bold-headers/style.css
//go:embed ats-two-line-header/template.html ats-two-line-header/style.css ats-skills-first/template.html ats-skills-first/style.css
//go:embed ats-academic/template.html ats-academic/style.css
//go:embed modern-banner/template.html modern-banner/style.css modern-timeline/template.html modern-timeline/style.css
//go:embed creative-accent/template.html creative-accent/style.css creative-bold/template.html creative-bold/style.css
//go:embed executive-elegant/template.html executive-elegant/style.css executive-serif-bold/template.html executive-serif-bold/style.css
//go:embed minimalist-lines/template.html minimalist-lines/style.css minimalist-airy/template.html minimalist-airy/style.css
//go:embed modern-gradient/template.html modern-gradient/style.css modern-split-header/template.html modern-split-header/style.css
//go:embed modern-card/template.html modern-card/style.css modern-dark-header/template.html modern-dark-header/style.css
//go:embed modern-dots/template.html modern-dots/style.css modern-photo-hero/template.html modern-photo-hero/style.css
//go:embed creative-pastel/template.html creative-pastel/style.css creative-monogram/template.html creative-monogram/style.css
//go:embed creative-duotone/template.html creative-duotone/style.css creative-stripe/template.html creative-stripe/style.css
//go:embed creative-sunset/template.html creative-sunset/style.css executive-navy/template.html executive-navy/style.css
//go:embed executive-monogram-rule/template.html executive-monogram-rule/style.css executive-charcoal/template.html executive-charcoal/style.css
//go:embed minimalist-photo-left/template.html minimalist-photo-left/style.css minimalist-whitespace/template.html minimalist-whitespace/style.css
//go:embed minimalist-mono-caps/template.html minimalist-mono-caps/style.css classic-portrait/template.html classic-portrait/style.css
//go:embed classic-ribbon/template.html classic-ribbon/style.css classic-centered-photo/template.html classic-centered-photo/style.css
//go:embed modern-slate-line/template.html modern-slate-line/style.css modern-indigo-strip/template.html modern-indigo-strip/style.css
//go:embed modern-ocean-banner/template.html modern-ocean-banner/style.css modern-berry-tint/template.html modern-berry-tint/style.css
//go:embed modern-emerald-timeline/template.html modern-emerald-timeline/style.css modern-copper-right/template.html modern-copper-right/style.css
//go:embed modern-sky-hero/template.html modern-sky-hero/style.css modern-graphite-frame/template.html modern-graphite-frame/style.css
//go:embed creative-plum-duo/template.html creative-plum-duo/style.css creative-amber-overlap/template.html creative-amber-overlap/style.css
//go:embed creative-rose-rule/template.html creative-rose-rule/style.css creative-teal-tint/template.html creative-teal-tint/style.css
//go:embed creative-olive-banner/template.html creative-olive-banner/style.css creative-violet-center/template.html creative-violet-center/style.css
//go:embed executive-crimson-center/template.html executive-crimson-center/style.css executive-navy-rule/template.html executive-navy-rule/style.css
//go:embed executive-gold-dark/template.html executive-gold-dark/style.css executive-forest-frame/template.html executive-forest-frame/style.css
//go:embed executive-steel-strip/template.html executive-steel-strip/style.css executive-charcoal-right/template.html executive-charcoal-right/style.css
//go:embed minimalist-plain-left/template.html minimalist-plain-left/style.css minimalist-plain-center/template.html minimalist-plain-center/style.css
//go:embed minimalist-line-right/template.html minimalist-line-right/style.css minimalist-sage-rule/template.html minimalist-sage-rule/style.css
//go:embed minimalist-steel-strip/template.html minimalist-steel-strip/style.css minimalist-sand-frame/template.html minimalist-sand-frame/style.css
//go:embed classic-navy-right/template.html classic-navy-right/style.css classic-berry-center/template.html classic-berry-center/style.css
//go:embed classic-ink-left/template.html classic-ink-left/style.css classic-forest-strip/template.html classic-forest-strip/style.css
//go:embed classic-gold-tint/template.html classic-gold-tint/style.css classic-crimson-rule/template.html classic-crimson-rule/style.css
//go:embed side-navy/template.html side-navy/style.css side-sage/template.html side-sage/style.css
//go:embed side-graphite-right/template.html side-graphite-right/style.css side-coral/template.html side-coral/style.css
//go:embed side-slate-wide/template.html side-slate-wide/style.css side-sand-serif/template.html side-sand-serif/style.css
//go:embed side-teal-banner/template.html side-teal-banner/style.css side-night-mono/template.html side-night-mono/style.css
//go:embed clean-blue-sidebar/template.html clean-blue-sidebar/style.css corporate-gold-sidebar/template.html corporate-gold-sidebar/style.css
//go:embed contemporary-navy-sidebar/template.html contemporary-navy-sidebar/style.css maker-dark-sidebar/template.html maker-dark-sidebar/style.css
//go:embed refined-red-sidebar/template.html refined-red-sidebar/style.css leadership-brown-sidebar/template.html leadership-brown-sidebar/style.css
//go:embed green-gradient-sidebar/template.html green-gradient-sidebar/style.css bold-blue-sidebar/template.html bold-blue-sidebar/style.css
//go:embed midnight-gold-sidebar/template.html midnight-gold-sidebar/style.css grey-pro-sidebar/template.html grey-pro-sidebar/style.css
//go:embed friendly-orange-sidebar/template.html friendly-orange-sidebar/style.css photo-panel-steel/template.html photo-panel-steel/style.css
//go:embed paint-splash/template.html paint-splash/style.css square-teal/template.html square-teal/style.css
//go:embed orange-frame/template.html orange-frame/style.css dotted-lime/template.html dotted-lime/style.css
//go:embed night-mode/template.html night-mode/style.css gilded-frame/template.html gilded-frame/style.css
//go:embed green-colorful/template.html green-colorful/style.css classic-red-columns/template.html classic-red-columns/style.css
//go:embed rich-gold-cream/template.html rich-gold-cream/style.css retro-green-double/template.html retro-green-double/style.css
//go:embed gradient-blue-diagonal/template.html gradient-blue-diagonal/style.css skills-first-orange/template.html skills-first-orange/style.css
var templateFS embed.FS

// IsKnownTemplate is exported so callers that need to validate a
// caller-submitted templateID WITHOUT actually rendering (e.g.
// jobs/cv_pdf.go's own save_cv_document, defensively re-checking a
// value it trusts was already used successfully by an earlier
// render_cv_document call in the same conversation) don't have to
// duplicate this list.
func IsKnownTemplate(id string) bool {
	for _, m := range metas {
		if m.ID == id {
			return true
		}
	}
	return false
}

// xmlDeclaration is prepended in Go code, AFTER html/template has
// already rendered the rest of the document — never written literally
// inside a .html.tmpl file. html/template parses its input as an HTML5
// document tree and re-serializes it; a leading "<?xml ...?>" appears
// before any real tag, so its own tokenizer treats it as a stray text
// node and HTML-escapes it on the way back out ("&lt;?xml...&gt;"),
// which would render as literal garbage text at the top of the PDF
// instead of a real (harmless, Chrome-ignored) XML declaration. The
// existing, already-proven AI-driven flow
// (buildGenerateCvMessage.ts's own XHTML_SKELETON) sends this exact
// declaration as a plain, non-templated string — this constant
// reproduces the same final byte sequence, just assembled at a point
// html/template's own parser never sees it. Confirmed by rendering
// and inspecting actual output before this fix was added.
const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// styleMarker is the exact placeholder every template.html's own
// <style> element contains — Render below replaces it with that same
// template's own style.css content, spliced in AFTER html/template has
// already rendered the rest of the document (never fed through
// html/template itself), for the identical reason xmlDeclaration above
// is assembled outside the parser too: html/template re-serializes its
// input as an HTML5 tree, and raw CSS routed through {{.CSS}} would
// either get HTML-escaped (mangling it — the exact same class of bug
// xmlDeclaration's own doc comment describes) or need a
// template.CSS-typed field to opt out of that escaping, which is more
// machinery than a plain string.Replace on a fixed marker needs.
const styleMarker = "<!--STYLE-->"

// renderContext embeds the caller-supplied CvData (so every existing
// template field, e.g. {{.FullName}}, keeps resolving exactly as
// before via Go's normal promoted-field lookup) alongside a photo the
// caller resolved separately. PhotoDataURI is deliberately NOT a field
// of CvData itself: CvData round-trips through the browser (the CV
// Builder wizard edits and re-submits it whole, and it's stored
// verbatim as data_json) — if the photo lived there, a client could
// submit an arbitrary string claiming to be the profile's photo, and
// typing it template.URL to make it render (see below) would render
// that value unvalidated. Keeping it a Render-only parameter means the
// caller (cvbuilder/handler.go) is the only place that can ever set it,
// always resolved server-side from the actual persona's own current
// profile photo — never client-supplied. See
// plan/ai/career/profile-image/step-02-cv-template-integration.md.
type renderContext struct {
	CvData
	// template.URL, not string: html/template's own contextual escaper
	// treats a plain string src="{{.}}" through its "safe URL" filter,
	// which only allow-lists http(s)/mailto schemes and would replace a
	// data: URI with "#ZgotmplZ" — the exact same class of escaping
	// gotcha xmlDeclaration/styleMarker above already ran into for XML
	// declarations and raw CSS. template.URL is how html/template's own
	// API says "this value is already a safe URL, don't filter it" —
	// safe here because it's never attacker-controlled (see above), only
	// ever a data: URI this package base64-encoded itself from Media's
	// own resized/re-encoded image bytes.
	PhotoDataURI template.URL
}

// Render renders data into XHTML using templateID's own design.
// html/template (NOT text/template) is what makes this safe to call
// with real user-entered text (company names, descriptions, summaries,
// all free text the caller never controls the shape of) — its
// automatic contextual escaping is the only thing standing between a
// user's own CV text and malformed or injected markup inside the
// XHTML pdf_tools goes on to render. An unknown templateID returns an
// error — never a silent fallback to some default design.
//
// photoDataURI is a ready-to-use "data:<content-type>;base64,<...>"
// string, or "" if this persona's own profile has no photo uploaded —
// resolved by the caller (cvbuilder/handler.go), never by this
// package, which has no HTTP client of its own and no notion of a
// profile.
func Render(templateID string, data CvData, photoDataURI string) (string, error) {
	if !IsKnownTemplate(templateID) {
		return "", fmt.Errorf("unknown template %q", templateID)
	}

	tmpl, err := template.ParseFS(templateFS, templateID+"/template.html")
	if err != nil {
		return "", err
	}

	ctx := renderContext{CvData: data, PhotoDataURI: template.URL(photoDataURI)}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}

	css, err := templateFS.ReadFile(templateID + "/style.css")
	if err != nil {
		return "", err
	}

	rendered := strings.Replace(buf.String(), styleMarker, string(css), 1)
	return xmlDeclaration + rendered, nil
}
