package httpapi

import (
	"fmt"
	"hash/fnv"
)

// topicColor assigns a stable display color to a dynamic topic. The taxonomy
// grows over time (CTFG-28), so colors can't be pinned to a fixed list — design
// requires "palette-assigned colors as topics emerge" that stay consistent for
// a given topic. We derive the hue from a hash of the topic name, holding
// saturation and lightness fixed (the design's "shared chroma/lightness" ramp),
// so the same topic always maps to the same warm-but-distinct hue regardless of
// what else exists. An empty topic gets a neutral gray.
func topicColor(topic string) string {
	if topic == "" {
		return "hsl(40, 8%, 60%)"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(topic))
	hue := int(h.Sum32() % 360)
	return fmt.Sprintf("hsl(%d, 62%%, 56%%)", hue)
}
