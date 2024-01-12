package main

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"

	"github.com/oxisto/owl2protobuf/internal/fileHeaders"
	"github.com/oxisto/owl2protobuf/internal/util"
	"github.com/oxisto/owl2protobuf/pkg/owl"
	"github.com/oxisto/owl2protobuf/pkg/protobuf"

	"github.com/lmittmann/tint"
)

// TODOs
// - get label instead of iri for the name fields
// - add comments
// - add relationsships
// - add ObjectSomeValuesFrom
// - add ObjectHasValue

var (
	file string
)

const (
	DefaultRootResourceName = "http://graph.clouditor.io/classes/CloudResource"
	DefaultOutputFile       = "output/ontology.proto"
)

// prepareOntology extracts important information from the owl ontology file that is needed for the protobuf file creation
func prepareOntology(o owl.Ontology) protobuf.OntologyPrepared {
	preparedOntology := protobuf.OntologyPrepared{
		Resources:           make(map[string]*protobuf.Resource),
		SubClasses:          make(map[string]owl.SubClassOf),
		AnnotationAssertion: make(map[string]owl.AnnotationAssertion),
	}

	// Prepare ontology classes
	// We set the name extracted from the IRI and the IRI. If a name label exists we will change the name later.
	for _, c := range o.Declarations {
		if c.Class.IRI != "" {
			preparedOntology.Resources[c.Class.IRI] = &protobuf.Resource{
				Iri:  c.Class.IRI,
				Name: util.GetNameFromIri(c.Class.IRI),
			}
		}
	}

	// Prepare name and comment
	for _, aa := range o.AnnotationAssertion {
		if aa.AnnotationProperty.AbbreviatedIRI == "rdfs:label" {
			if _, ok := preparedOntology.Resources[aa.IRI]; ok {
				preparedOntology.Resources[aa.IRI].Name = util.CleanString(aa.Literal)

			}
		} else if aa.AnnotationProperty.AbbreviatedIRI == "rdfs:comment" {
			if _, ok := preparedOntology.Resources[aa.IRI]; ok {
				c := preparedOntology.Resources[aa.IRI].Comment
				c = append(c, aa.Literal)
				preparedOntology.Resources[aa.IRI].Comment = c

			}
		}
	}

	// Prepare SubClasses
	// There are 3 different structures of SubClasses. All Class properties are IRIs:
	// - 2 Classes: The second Class is the parent of the first Class
	// - Class and DataSomeValuesFrom: Class is the current resource and DataSomeValuesFrom contains the Datatype (e.g., xsd:string) and the corresponding DataProperty/variable name as IRI or abbreviatedIRI (e.g., filename as IRI or prop:enabeld as abbreviatedIRI)
	// - Class and ObjectSomeValuesFrom: Class is the current resource and ObjectSomeValuesFrom contains the ObjectProperty (e.g., prop:hasMultiple) and the linked resource (Class)
	for _, sc := range o.SubClasses {
		if len(sc.Class) == 2 {

			// "owl.Thing" is the root of the ontology and is not needed for the protobuf files
			if sc.Class[1].IRI != "owl.Thing" {
				// Create resource that has a parent. All resources directly under "owl.Thing" are alread created before (via the Declarations)
				r := &protobuf.Resource{
					Iri:     sc.Class[0].IRI,
					Name:    preparedOntology.Resources[sc.Class[0].IRI].Name,
					Parent:  sc.Class[1].IRI,
					Comment: preparedOntology.Resources[sc.Class[0].IRI].Comment,
				}

				// Add subresources to the parent resource
				if val, ok := preparedOntology.Resources[sc.Class[1].IRI]; ok {
					if val.SubResources == nil {
						preparedOntology.Resources[sc.Class[1].IRI].SubResources = make([]*protobuf.Resource, 0)
					}
					preparedOntology.Resources[sc.Class[1].IRI].SubResources = append(preparedOntology.Resources[sc.Class[1].IRI].SubResources, r)
				}

				// Add parent IRI to resource (not subresource!). We couldn't do this beforehand (Declarations) because we only get the information here,
				preparedOntology.Resources[sc.Class[0].IRI].Parent = sc.Class[1].IRI
			}
		} else if sc.DataSomeValuesFrom != nil {
			// Add data values, e.g. "enabled xsd:bool"
			for _, v := range sc.DataSomeValuesFrom {
				preparedOntology.Resources[sc.Class[0].IRI].Relationship = append(preparedOntology.Resources[sc.Class[0].IRI].Relationship, &protobuf.Relationship{
					Typ:   util.GetGoType(v.Datatype.AbbreviatedIRI),
					Value: util.GetDataPropertyName(v.DataProperty.AbbreviatedIRI),
				})
			}
		} else if sc.ObjectSomeValuesFrom != nil {
			// Add object values, e.g., "offers ResourceLogging"
			for _, v := range sc.ObjectSomeValuesFrom {
				preparedOntology.Resources[sc.Class[0].IRI].ObjectRelationship = append(preparedOntology.Resources[sc.Class[0].IRI].ObjectRelationship, &protobuf.ObjectRelationship{
					ObjectProperty: v.ObjectProperty.AbbreviatedIRI,
					Class:          v.Class.IRI,
					Name:           preparedOntology.Resources[v.Class.IRI].Name,
				})
			}
		}
	}

	return preparedOntology
}

// createProtoFile creates the protobuf file
func createProtoFile(preparedOntology protobuf.OntologyPrepared) string {
	output := ""

	//Add header
	// TODO(all): Another header for other tools?
	output += fileHeaders.GetClouditorProtobufFileDetails()

	// Create proto messages with comments
	for _, v := range preparedOntology.Resources {
		// is the counter for the message field numbers
		i := 0

		// Add comment
		for _, v := range v.Comment {
			output += "\n// " + v
		}

		// Start message
		output += fmt.Sprintf("\nmessage %s {", v.Name)

		// Add data properties
		for _, r := range v.Relationship {
			if r.Typ != "" && r.Value != "" {
				i += 1
				output += fmt.Sprintf("\n\t%s %s  = %d;", r.Value, r.Typ, i)
			}
		}

		// Add object properties
		for _, o := range v.ObjectRelationship {
			if o.Name != "" && o.ObjectProperty != "" {
				i += 1
				value, typ := util.GetObjectDetail(o.ObjectProperty, DefaultRootResourceName, preparedOntology.Resources[o.Class], preparedOntology)
				output += fmt.Sprintf("\n\t%s%s %s  = %d;", value, o.Name, typ, i)
			}
		}

		// Add subresources if present
		// Important
		if len(v.SubResources) > 0 {
			// j is the counter for the oneof field numbers
			j := 0
			output += "\n\n\toneof type {"
			for _, v2 := range v.SubResources {
				j += 1

				output += fmt.Sprintf("\n\t\t%s %s = %d;", v2.Name, util.ToSnakeCase(v2.Name), j)

			}
			output += "\n\t}"
		}

		// Close message
		output += "\n}\n"
	}

	return output

}

func writeProtofileToStorage(s string) error {
	var err error

	// Create storage file
	f, err := os.Create(DefaultOutputFile)
	if err != nil {
		err = fmt.Errorf("error creating file: %v", err)
		slog.Error(err.Error())
	}

	// Write output string to file
	_, err = f.WriteString(s)
	if err != nil {
		err = fmt.Errorf("error writing output to file: %v", err)
		slog.Error(err.Error())
		f.Close()
		return err
	}

	// Close storage file
	err = f.Close()
	if err != nil {
		err = fmt.Errorf("error closing file: %v", err)
		slog.Error(err.Error())
		return err
	}

	return nil
}

func main() {
	var (
		b   []byte
		err error
		o   owl.Ontology
	)

	file = os.Args[1]

	// Set up logging
	slog.SetDefault(slog.New(
		tint.NewHandler(os.Stdout, &tint.Options{
			Level: slog.LevelDebug,
		}),
	))

	// Read XML
	b, err = os.ReadFile(file)
	if err != nil {
		slog.Error("error reading file", tint.Err(err))
		return
	}

	err = xml.Unmarshal(b, &o)
	if err != nil {
		slog.Error("error while unmarshalling XML", tint.Err(err))
		return
	}
	// fmt.Printf("%+v", o)

	// prepareOntology
	preparedOntology := prepareOntology(o)

	// Generate protobuf file
	output := createProtoFile(preparedOntology)

	err = writeProtofileToStorage(output)
	if err != nil {
		slog.Error("error writing protobuf file to storage", tint.Err(err))
	}

	slog.Info("protobuf file written to storage", slog.String("output folder", DefaultOutputFile))
}
