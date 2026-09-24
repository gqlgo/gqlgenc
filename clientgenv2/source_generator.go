package clientgenv2

import (
	"fmt"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/templates"

	gqlgencConfig "github.com/gqlgo/gqlgenc/config"

	"github.com/vektah/gqlparser/v2/ast"
)

type Argument struct {
	Variable string
	Type     types.Type
}

type ResponseField struct {
	Name             string
	IsFragmentSpread bool
	IsInlineFragment bool
	Type             types.Type
	Tags             []string
	ResponseFields   ResponseFieldList
}

func (r ResponseField) FieldTypeString() string {
	fullFieldType := r.Type.String()
	parts := strings.Split(fullFieldType, ".")

	return parts[len(parts)-1]
}

type ResponseFieldList []*ResponseField

func (rs ResponseFieldList) IsFragmentSpread() bool {
	if len(rs) != 1 {
		return false
	}

	return rs[0].IsFragmentSpread
}

func (rs ResponseFieldList) StructType() *types.Struct {
	vars := make([]*types.Var, 0, len(rs))
	structTags := make([]string, 0, len(rs))

	for _, field := range rs {
		vars = append(vars, types.NewVar(0, nil, templates.ToGo(field.Name), field.Type))
		structTags = append(structTags, strings.Join(field.Tags, " "))
	}

	return types.NewStruct(vars, structTags)
}

func (rs ResponseFieldList) IsFragment() bool {
	if len(rs) != 1 {
		return false
	}

	return rs[0].IsInlineFragment || rs[0].IsFragmentSpread
}

func (rs ResponseFieldList) IsBasicType() bool {
	return len(rs) == 0
}

func (rs ResponseFieldList) IsStructType() bool {
	return len(rs) > 0 && !rs.IsFragment()
}

func (rs ResponseFieldList) MapByName() map[string]*ResponseField {
	res := make(map[string]*ResponseField)
	for _, field := range rs {
		res[field.Name] = field
	}

	return res
}

func (rs ResponseFieldList) SortByName() ResponseFieldList {
	slices.SortFunc(rs, func(a, b *ResponseField) int {
		return strings.Compare(a.Name, b.Name)
	})

	return rs
}

type StructGenerator struct {
	// Create fields based on this ResponseFieldList
	currentResponseFieldList ResponseFieldList
	// Struct sources that will no longer be created due to merging
	preMergedStructSources []*StructSource
	// Struct sources that will be created due to merging
	postMergedStructSources []*StructSource
}

func NewStructGenerator(responseFieldList ResponseFieldList) *StructGenerator {
	currentFields := make(ResponseFieldList, 0)
	fragmentChildrenFields := make(ResponseFieldList, 0)

	for _, field := range responseFieldList {
		if field.IsFragmentSpread {
			fragmentChildrenFields = append(fragmentChildrenFields, field.ResponseFields...)
		} else {
			currentFields = append(currentFields, field)
		}
	}

	preMergedStructSources := make([]*StructSource, 0)

	for _, field := range responseFieldList {
		if field.IsFragmentSpread {
			preMergedStructSources = append(preMergedStructSources, &StructSource{
				Name: field.FieldTypeString(),
				Type: field.ResponseFields.StructType(),
			})
		}
	}

	currentFields, preMergedStructSources, postMergedStructSources := mergeFieldsRecursively(currentFields, fragmentChildrenFields, preMergedStructSources, nil)

	return &StructGenerator{
		currentResponseFieldList: currentFields,
		preMergedStructSources:   preMergedStructSources,
		postMergedStructSources:  postMergedStructSources,
	}
}

func mergeFieldsRecursively(targetFields, sourceFields ResponseFieldList, preMerged, postMerged []*StructSource) (ResponseFieldList, []*StructSource, []*StructSource) {
	responseFieldList := make(ResponseFieldList, 0)
	targetFieldsMap := targetFields.MapByName()
	newPreMerged := preMerged
	newPostMerged := postMerged

	for _, sourceField := range sourceFields {
		if targetField, ok := targetFieldsMap[sourceField.Name]; ok {
			if targetField.ResponseFields.IsBasicType() {
				continue
			}

			newPreMerged = append(newPreMerged, &StructSource{
				Name: sourceField.FieldTypeString(),
				Type: sourceField.ResponseFields.StructType(),
			})
			newPreMerged = append(newPreMerged, &StructSource{
				Name: targetField.FieldTypeString(),
				Type: targetField.ResponseFields.StructType(),
			})

			targetField.ResponseFields, newPreMerged, newPostMerged = mergeFieldsRecursively(targetField.ResponseFields, sourceField.ResponseFields, newPreMerged, newPostMerged)

			newPostMerged = append(newPostMerged, &StructSource{
				Name: targetField.FieldTypeString(),
				Type: targetField.ResponseFields.StructType(),
			})
		} else {
			targetFieldsMap[sourceField.Name] = sourceField
		}
	}

	for _, field := range targetFieldsMap {
		responseFieldList = append(responseFieldList, field)
	}

	responseFieldList = responseFieldList.SortByName()

	return responseFieldList, newPreMerged, newPostMerged
}

func structSourcesMapByTypeName(sources []*StructSource) map[string]*StructSource {
	res := make(map[string]*StructSource)
	for _, source := range sources {
		res[source.Name] = source
	}

	return res
}

func (g *StructGenerator) MergedStructSources(sources []*StructSource) []*StructSource {
	preMergedStructSourcesMap := structSourcesMapByTypeName(g.preMergedStructSources)
	res := make([]*StructSource, 0)
	// remove pre-merged struct
	for _, source := range sources {
		// when name is same, remove it
		if _, ok := preMergedStructSourcesMap[source.Name]; ok {
			continue
		}

		res = append(res, source)
	}

	// append post-merged struct
	res = append(res, g.postMergedStructSources...)

	return res
}

func (g *StructGenerator) GetCurrentResponseFieldList() ResponseFieldList {
	return g.currentResponseFieldList
}

type StructSource struct {
	Name string
	Type types.Type
}

type SourceGenerator struct {
	cfg            *config.Config
	binder         *config.Binder
	client         config.PackageConfig
	generateConfig *gqlgencConfig.GenerateConfig
	StructSources  []*StructSource
}

func NewSourceGenerator(cfg *config.Config, client config.PackageConfig, generateConfig *gqlgencConfig.GenerateConfig) *SourceGenerator {
	if generateConfig == nil {
		generateConfig = new(gqlgencConfig.GenerateConfig)
	}
	return &SourceGenerator{
		cfg:            cfg,
		binder:         cfg.NewBinder(),
		client:         client,
		generateConfig: generateConfig,
		StructSources:  []*StructSource{},
	}
}

func (r *SourceGenerator) NewResponseFields(selectionSet ast.SelectionSet, typeName string) ResponseFieldList {
	responseFields := make(ResponseFieldList, 0, len(selectionSet))
	for _, selection := range selectionSet {
		responseFields = append(responseFields, r.NewResponseField(selection, typeName))
	}

	return responseFields
}

func NewLayerTypeName(base, thisField string) string {
	return fmt.Sprintf("%s_%s", cases.Title(language.Und, cases.NoLower).String(base), thisField)
}

// NewResponseField converts one selection into the field of the generated response struct.
//
// Arguments:
//   - selection: an *ast.Field, *ast.FragmentSpread, or *ast.InlineFragment
//   - typeName: the name of the struct the field belongs to; nested struct names are derived from it
//
// Returns:
//   - *ResponseField: the field, with its Go type and the fields of its own selection set
//
// Preconditions:
//   - selection is one of the three selection kinds; anything else is a bug and panics
//
// Postconditions:
//   - struct types needed by the field are appended to r.StructSources
func (r *SourceGenerator) NewResponseField(selection ast.Selection, typeName string) *ResponseField {
	switch selection := selection.(type) {
	case *ast.Field:
		return r.newFieldResponseField(selection, typeName)
	case *ast.FragmentSpread:
		return r.newFragmentSpreadResponseField(selection, typeName)
	case *ast.InlineFragment:
		return r.newInlineFragmentResponseField(selection, typeName)
	}

	panic("unexpected selection type")
}

func (r *SourceGenerator) OperationArguments(variableDefinitions ast.VariableDefinitionList) []*Argument {
	argumentTypes := make([]*Argument, 0, len(variableDefinitions))
	for _, v := range variableDefinitions {
		argumentTypes = append(argumentTypes, &Argument{
			Variable: v.Variable,
			Type:     r.binder.CopyModifiersFromAst(v.Type, r.Type(v.Type.Name())),
		})
	}

	return argumentTypes
}

// Typeの引数に渡すtypeNameは解析した結果からselectionなどから求めた型の名前を渡さなければいけない
func (r *SourceGenerator) Type(typeName string) types.Type {
	goType, err := r.binder.FindTypeFromName(r.cfg.Models[typeName].Model[0])
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	return goType
}

// newFieldResponseField builds the response field for a plain field selection.
//
// Arguments:
//   - selection: the field selection
//   - typeName: the name of the enclosing struct
//
// Returns:
//   - *ResponseField: the field with json and graphql tags
//
// Preconditions:
//   - selection.Definition is resolved
//
// Postconditions:
//   - a struct type is appended to r.StructSources when the field has an object selection set
func (r *SourceGenerator) newFieldResponseField(selection *ast.Field, typeName string) *ResponseField {
	typeName = NewLayerTypeName(typeName, templates.ToGo(selection.Alias))
	fieldsResponseFields := r.NewResponseFields(selection.SelectionSet, typeName)

	baseType := r.fieldBaseType(selection, typeName, fieldsResponseFields)

	// return pointer type then optional type or slice pointer then slice type of definition in GraphQL.
	typ := r.binder.CopyModifiersFromAst(selection.Definition.Type, baseType)

	isOptional := !selection.Definition.Type.NonNull

	return &ResponseField{
		Name:           selection.Alias,
		Type:           typ,
		Tags:           r.fieldTags(selection.Alias, isOptional),
		ResponseFields: fieldsResponseFields,
	}
}

// fieldBaseType resolves the Go type of a field before GraphQL modifiers are applied.
//
// Arguments:
//   - selection: the field selection
//   - typeName: the name to use for a generated struct type
//   - fieldsResponseFields: the fields of the selection set of the field
//
// Returns:
//   - types.Type: the bound scalar or enum type, the fragment type, or a generated struct type
//
// Preconditions:
//   - fieldsResponseFields is a basic type, a single fragment, or a struct; anything else is a bug and panics
//
// Postconditions:
//   - for a struct, the merged struct sources and the new struct are recorded in r.StructSources
func (r *SourceGenerator) fieldBaseType(selection *ast.Field, typeName string, fieldsResponseFields ResponseFieldList) types.Type {
	switch {
	case fieldsResponseFields.IsBasicType():
		return r.Type(selection.Definition.Type.Name())
	case fieldsResponseFields.IsFragment():
		// if a child field is fragment, this field type became fragment.
		return fieldsResponseFields[0].Type
	case fieldsResponseFields.IsStructType():
		// if there is a fragment in child fields, merge it with the current field
		generator := NewStructGenerator(fieldsResponseFields)

		// restruct struct sources
		r.StructSources = generator.MergedStructSources(r.StructSources)

		// append current struct
		structType := generator.GetCurrentResponseFieldList().StructType()
		r.StructSources = append(r.StructSources, &StructSource{
			Name: typeName,
			Type: structType,
		})

		return types.NewNamed(
			types.NewTypeName(0, r.client.Pkg(), typeName, nil),
			structType,
			nil,
		)
	default:
		// here is bug
		panic("not match type")
	}
}

// fieldTags builds the struct tags of a plain field.
//
// Arguments:
//   - alias: the response key of the field
//   - isOptional: whether the field is nullable in GraphQL
//
// Returns:
//   - []string: the json tag, with omitempty and omitzero when configured for optional fields, and the graphql tag
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (r *SourceGenerator) fieldTags(alias string, isOptional bool) []string {
	jsonTag := fmt.Sprintf(`json:"%s`, alias)

	if isOptional {
		if r.generateConfig.EnableClientJsonOmitemptyTag != nil && *r.generateConfig.EnableClientJsonOmitemptyTag {
			jsonTag += `,omitempty`
		}

		if r.generateConfig.EnableClientJsonOmitzeroTag != nil && *r.generateConfig.EnableClientJsonOmitzeroTag {
			jsonTag += `,omitzero`
		}
	}

	jsonTag += `"`

	return []string{
		jsonTag,
		fmt.Sprintf(`graphql:"%s"`, alias),
	}
}

// newFragmentSpreadResponseField builds the response field for a fragment spread.
//
// Arguments:
//   - selection: the fragment spread
//   - typeName: the name of the enclosing struct
//
// Returns:
//   - *ResponseField: a field marked IsFragmentSpread; it is not rendered by the template but
//     lets the parent detect and merge the fragment
//
// Preconditions:
//   - selection.Definition is resolved
//
// Postconditions:
//   - none
func (r *SourceGenerator) newFragmentSpreadResponseField(selection *ast.FragmentSpread, typeName string) *ResponseField {
	fieldsResponseFields := r.NewResponseFields(selection.Definition.SelectionSet, NewLayerTypeName(typeName, templates.ToGo(selection.Name)))
	baseType := types.NewNamed(
		types.NewTypeName(0, r.client.Pkg(), templates.ToGo(selection.Name), nil),
		fieldsResponseFields.StructType(),
		nil,
	)

	var typ types.Type = baseType
	if r.cfg.StructFieldsAlwaysPointers {
		typ = types.NewPointer(baseType)
	}

	return &ResponseField{
		Name:             selection.Name,
		Type:             typ,
		IsFragmentSpread: true,
		ResponseFields:   fieldsResponseFields,
	}
}

// newInlineFragmentResponseField builds the response field for an inline fragment.
//
// Arguments:
//   - selection: the inline fragment
//   - typeName: the name of the enclosing struct
//
// Returns:
//   - *ResponseField: a field tagged with "... on TypeCondition" whose type is the fragment struct
//
// Preconditions:
//   - selection.TypeCondition is set
//
// Postconditions:
//   - a struct type named after the type condition is appended to r.StructSources, unless the
//     selection set is a single fragment spread, whose type is reused instead
func (r *SourceGenerator) newInlineFragmentResponseField(selection *ast.InlineFragment, typeName string) *ResponseField {
	// InlineFragment has child elements, so create a struct type here
	name := NewLayerTypeName(typeName, templates.ToGo(selection.TypeCondition))
	fieldsResponseFields := r.NewResponseFields(selection.SelectionSet, name)

	// if single fields that is also a fragment spread, reuse that fragment
	if len(fieldsResponseFields) == 1 && fieldsResponseFields[0].IsFragmentSpread {
		baseType := types.NewNamed(
			types.NewTypeName(0, r.client.Pkg(), templates.ToGo(fieldsResponseFields[0].Name), nil),
			fieldsResponseFields.StructType(),
			nil,
		)

		return &ResponseField{
			Name:           selection.TypeCondition,
			Type:           r.inlineFragmentType(baseType),
			Tags:           inlineFragmentTags(selection.TypeCondition),
			ResponseFields: fieldsResponseFields,
		}
	}

	fields := fieldsResponseFields
	if r.hasFragmentSpread(fieldsResponseFields) {
		// collect all fields from fragment: the own fields first, then the fields of the spreads
		fields = make(ResponseFieldList, 0, len(fieldsResponseFields))

		for _, field := range fieldsResponseFields {
			if !field.IsFragmentSpread {
				fields = append(fields, field)
			}
		}

		fields = append(fields, r.collectFragmentFields(fieldsResponseFields)...)
	}

	// generate struct
	structType := fields.StructType()
	r.StructSources = append(r.StructSources, &StructSource{
		Name: name,
		Type: structType,
	})
	baseType := types.NewNamed(
		types.NewTypeName(0, r.client.Pkg(), name, nil),
		structType,
		nil,
	)

	return &ResponseField{
		Name:             selection.TypeCondition,
		Type:             r.inlineFragmentType(baseType),
		IsInlineFragment: true,
		Tags:             inlineFragmentTags(selection.TypeCondition),
		ResponseFields:   fields.SortByName(),
	}
}

// inlineFragmentType applies the inlineFragmentAlwaysPointers option to a fragment struct type.
//
// Arguments:
//   - baseType: the fragment struct type
//
// Returns:
//   - types.Type: a pointer to baseType when the option is enabled, otherwise baseType
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (r *SourceGenerator) inlineFragmentType(baseType types.Type) types.Type {
	if r.generateConfig.InlineFragmentAlwaysPointers != nil && *r.generateConfig.InlineFragmentAlwaysPointers {
		return types.NewPointer(baseType)
	}

	return baseType
}

// inlineFragmentTags builds the struct tags of an inline fragment field.
//
// Arguments:
//   - typeCondition: the type the fragment applies to
//
// Returns:
//   - []string: the graphql tag "... on <typeCondition>"
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func inlineFragmentTags(typeCondition string) []string {
	return []string{fmt.Sprintf(`graphql:"... on %s"`, typeCondition)}
}

func (r *SourceGenerator) hasFragmentSpread(fields ResponseFieldList) bool {
	for _, field := range fields {
		if field.IsFragmentSpread {
			return true
		}
	}

	return false
}

func (r *SourceGenerator) collectFragmentFields(fields ResponseFieldList) ResponseFieldList {
	var fragmentFields ResponseFieldList

	for _, field := range fields {
		if field.IsFragmentSpread {
			fragmentFields = append(fragmentFields, field.ResponseFields...)
		}
	}

	return fragmentFields
}

// collectSpreadFragmentInfo extracts direct fragment spread info from a ResponseFieldList.
// Only collects the immediate spreads (not recursive), since each fragment generates
// its own conversion getters for its direct spreads, enabling chaining.
func collectSpreadFragmentInfo(fields ResponseFieldList) []*SpreadFragmentInfo {
	var result []*SpreadFragmentInfo

	for _, field := range fields {
		if field.IsFragmentSpread {
			result = append(result, &SpreadFragmentInfo{
				Name: field.Name,
				Type: field.Type,
			})
		}
	}

	return result
}

// flattenFragmentSpreads recursively replaces fragment spread fields with their
// child fields. This ensures that nested fragment spreads (e.g., FragA -> FragB -> FragC)
// are fully expanded before struct generation.
func flattenFragmentSpreads(fields ResponseFieldList) ResponseFieldList {
	result := make(ResponseFieldList, 0, len(fields))

	for _, field := range fields {
		if field.IsFragmentSpread {
			// Replace the spread with its children, recursively flattening
			expanded := flattenFragmentSpreads(field.ResponseFields)
			result = append(result, expanded...)
		} else {
			result = append(result, field)
		}
	}

	return result
}
