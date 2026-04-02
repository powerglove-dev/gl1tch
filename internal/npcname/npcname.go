// Package npcname maps a numeric run ID to a deterministic NPC-style name.
// The same ID always produces the same name; no storage required.
package npcname

var adjectives = []string{
	"Ashen", "Bitter", "Bone", "Brackish", "Cinder",
	"Corroded", "Crimson", "Crumbling", "Dark", "Dead",
	"Dire", "Dread", "Dust", "Ember", "Fallen",
	"Gaunt", "Grim", "Hollow", "Hunted", "Iron",
	"Jagged", "Kelp", "Leaden", "Marked", "Midnight",
	"Moss", "Murk", "Obsidian", "Pale", "Rusted",
	"Salt", "Scarred", "Shallow", "Silent", "Slag",
	"Smoke", "Stone", "Tallow", "Thorn", "Void",
}

var nouns = []string{
	"Acolyte", "Archer", "Archivist", "Asp", "Blade",
	"Brand", "Caller", "Cipher", "Cloak", "Construct",
	"Crawl", "Drifter", "Etcher", "Fang", "Finder",
	"Forge", "Gloom", "Grave", "Hand", "Herald",
	"Hollow", "Hunger", "Keeper", "Lancer", "Lurk",
	"Mender", "Oracle", "Pyre", "Rook", "Sage",
	"Scrawl", "Seeker", "Shade", "Shard", "Shroud",
	"Sigil", "Sorrow", "Spark", "Specter", "Warden",
}

// FromID returns a deterministic two-word NPC name for the given run ID.
func FromID(id int64) string {
	a := adjectives[id%int64(len(adjectives))]
	n := nouns[(id/int64(len(adjectives)))%int64(len(nouns))]
	return a + " " + n
}
