# Rule-based natural language parsing, not LLM-backed

A Query's natural language is parsed into a Filter using a local, fixed grammar of recognized phrases rather than by calling out to an LLM. We chose this deliberately, even though "natural language" often implies an LLM under the hood: Wheelman should work fully offline against any cluster, with no external API dependency, no API key requirement, and no per-query network round-trip or cost. A future reader tempted to "simplify" this by routing parsing through an LLM should know that offline operation was the point, not an oversight.
