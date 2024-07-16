package ontology

import (
	"strings"

	"github.com/oxisto/owl2proto/owl"
)

// NormalizedIRI returns the normalized (abbreviated) IRI
func (ont *OntologyPrepared) NormalizedIRI(c *owl.Entity) string {
	if c.IRI != "" {
		return c.IRI
	} else if c.AbbreviatedIRI != "" {
		return ont.normalizeAbbreviatedIRI(c.AbbreviatedIRI)
	}

	return ""
}

// normalizeAbbreviatedIRI normalizes the abbreviated IRI, e.g., "ex:Storage" -> "http://example.com/cloud/Storage"
func (ont *OntologyPrepared) normalizeAbbreviatedIRI(iri string) string {
	// We need to split the abbreviated IRI and look for the matching prefix
	prefix, name, found := strings.Cut(iri, ":")
	if !found {
		return iri
	}

	p, ok := ont.Prefixes[prefix]
	if ok {
		return p.IRI + name
	}

	return iri
}

// AbbreviateIRI returns an abbreviated IRI, e.g., "ex:Storage" -> "http://example.com/cloud/Storage" if a matching
// prefix is found. Otherwise, the long version is returned
func (ont *OntologyPrepared) AbbreviateIRI(iri string) string {
	for short, prefix := range ont.Prefixes {
		_, name, found := strings.Cut(iri, prefix.IRI)
		if found {
			return short + ":" + name
		}
	}

	return iri
}
