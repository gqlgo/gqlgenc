package clientgenv2

import (
	"errors"
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

	// StructTypeName is the name of the struct type a field with a selection set
	// gets when it cannot reuse a fragment type: the name of the enclosing struct
	// followed by the field name. It is empty for other fields.
	StructTypeName string
	// structFieldType builds the type of the field from a struct type named
	// StructTypeName, applying the list and non-null modifiers of the GraphQL field.
	// It is nil when StructTypeName is empty.
	structFieldType func(structType *types.Struct) types.Type
}

func (r ResponseField) FieldTypeString() string {
	fullFieldType := r.Type.String()
	parts := strings.Split(fullFieldType, ".")

	return parts[len(parts)-1]
}

// isFragmentTyped reports whether the field has the type of a fragment, which
// happens when its selection set is a single fragment spread.
//
// Arguments:
//   - none
//
// Returns:
//   - bool: true when the only selection of the field is a fragment spread
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (r ResponseField) isFragmentTyped() bool {
	return r.ResponseFields.IsFragmentSpread()
}

type ResponseFieldList []*ResponseField

func (rs ResponseFieldList) IsFragmentSpread() bool {
	if len(rs) != 1 {
		return false
	}

	return rs[0].IsFragmentSpread
}

// IsSameFragmentSpread reports whether rs and other each consist of a single
// spread of the same fragment.
//
// Arguments:
//   - other: the response fields to compare with
//
// Returns:
//   - bool: true when both lists are one fragment spread and the fragment names match
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (rs ResponseFieldList) IsSameFragmentSpread(other ResponseFieldList) bool {
	return rs.IsFragmentSpread() && other.IsFragmentSpread() && rs[0].Name == other[0].Name
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
			// a fragment may spread other fragments; their fields belong to the same struct
			fragmentChildrenFields = append(fragmentChildrenFields, flattenFragmentSpreads(field.ResponseFields)...)
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

			// Both fields already have the type of the same fragment, e.g. `meta { ...MetaDetails }`
			// selected in two fragments. Merging them would declare that fragment type again.
			if targetField.ResponseFields.IsSameFragmentSpread(sourceField.ResponseFields) {
				continue
			}

			// The struct types of both fields are replaced by the merged one. A field
			// typed as a fragment has no struct type of its own, and the fragment's
			// declaration must stay.
			for _, field := range []*ResponseField{sourceField, targetField} {
				if field.isFragmentTyped() {
					continue
				}

				newPreMerged = append(newPreMerged, &StructSource{
					Name: field.FieldTypeString(),
					Type: field.ResponseFields.StructType(),
				})
			}

			// A field typed as a fragment cannot keep that type once other fields are
			// merged into it, so it moves to a struct type of its own.
			mergedTypeName := targetField.FieldTypeString()
			if targetField.isFragmentTyped() && targetField.StructTypeName != "" {
				mergedTypeName = targetField.StructTypeName
			}

			// The merged struct has the fields of the spread fragments, not the spreads.
			targetField.ResponseFields, newPreMerged, newPostMerged = mergeFieldsRecursively(
				flattenFragmentSpreads(targetField.ResponseFields),
				flattenFragmentSpreads(sourceField.ResponseFields),
				newPreMerged, newPostMerged,
			)

			mergedStructType := targetField.ResponseFields.StructType()
			if targetField.structFieldType != nil && mergedTypeName == targetField.StructTypeName {
				targetField.Type = targetField.structFieldType(mergedStructType)
			}

			newPostMerged = append(newPostMerged, &StructSource{
				Name: mergedTypeName,
				Type: mergedStructType,
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

	// fieldNameCollisions records selection sets whose response keys map to the
	// same Go identifier; NewResponseFields cannot return an error itself.
	fieldNameCollisions []error
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
	// Check before building any field: NewResponseField builds the struct types of
	// nested selections eagerly, and go/types panics on duplicate field names.
	err := checkGoNameCollision(selectionSet, typeName)
	if err != nil {
		r.fieldNameCollisions = append(r.fieldNameCollisions, err)

		return nil
	}

	responseFields := make(ResponseFieldList, 0, len(selectionSet))
	for _, selection := range mergeDuplicateSelections(selectionSet) {
		responseFields = append(responseFields, r.NewResponseField(selection, typeName))
	}

	return responseFields
}

// mergeDuplicateSelections merges field selections that share a response key and
// inline fragments that apply to the same type, as a GraphQL server does when it
// collects fields.
//
// Arguments:
//   - selectionSet: the selections of one object
//
// Returns:
//   - ast.SelectionSet: the selections in their first-seen order, with the sub-selections
//     of later duplicates appended to the first occurrence; fragment spreads are kept as
//     they are
//
// Preconditions:
//   - selectionSet has been validated, so duplicate keys refer to the same field and
//     every inline fragment has its enclosing type resolved
//
// Postconditions:
//   - selectionSet itself is not modified; merged fields and inline fragments are copies
//   - every inline fragment in the result has a type condition; one written without a
//     type condition has the enclosing type
//   - no two inline fragments in the result have the same type condition
//   - a merged field has @skip or @include only when every occurrence of it has one
func mergeDuplicateSelections(selectionSet ast.SelectionSet) ast.SelectionSet {
	merged := make(ast.SelectionSet, 0, len(selectionSet))
	fieldByAlias := make(map[string]*ast.Field)
	inlineFragmentByTypeCondition := make(map[string]*ast.InlineFragment)

	for _, selection := range selectionSet {
		switch selection := selection.(type) {
		case *ast.Field:
			if first, seen := fieldByAlias[selection.Alias]; seen {
				first.SelectionSet = append(first.SelectionSet, selection.SelectionSet...)

				// One occurrence without @skip and @include is enough for the
				// server to always return the field.
				if !isConditional(selection.Directives) {
					first.Directives = withoutConditions(first.Directives)
				}

				continue
			}

			copied := *selection
			copied.SelectionSet = append(ast.SelectionSet(nil), selection.SelectionSet...)
			fieldByAlias[selection.Alias] = &copied
			merged = append(merged, &copied)
		case *ast.InlineFragment:
			typeCondition := inlineFragmentTypeCondition(selection)
			if first, seen := inlineFragmentByTypeCondition[typeCondition]; seen {
				first.SelectionSet = append(first.SelectionSet, selection.SelectionSet...)

				continue
			}

			copied := *selection
			copied.TypeCondition = typeCondition
			copied.SelectionSet = append(ast.SelectionSet(nil), selection.SelectionSet...)
			inlineFragmentByTypeCondition[typeCondition] = &copied
			merged = append(merged, &copied)
		default:
			merged = append(merged, selection)
		}
	}

	return merged
}

// isConditional reports whether directives can make a server leave a selection out of the response.
//
// Arguments:
//   - directives: the directives of one selection
//
// Returns:
//   - bool: true when directives contain @skip or @include
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func isConditional(directives ast.DirectiveList) bool {
	return directives.ForName("skip") != nil || directives.ForName("include") != nil
}

// withoutConditions removes @skip and @include from the directives of one selection.
//
// Arguments:
//   - directives: the directives of one selection
//
// Returns:
//   - ast.DirectiveList: the other directives, in their original order
//
// Preconditions:
//   - none
//
// Postconditions:
//   - directives itself is not modified
func withoutConditions(directives ast.DirectiveList) ast.DirectiveList {
	unconditional := make(ast.DirectiveList, 0, len(directives))

	for _, directive := range directives {
		if directive.Name != "skip" && directive.Name != "include" {
			unconditional = append(unconditional, directive)
		}
	}

	return unconditional
}

// FieldNameCollisionError returns the response keys that collide once converted to Go identifiers.
//
// Arguments:
//   - none
//
// Returns:
//   - error: nil if every selection set seen so far maps to distinct Go field names,
//     otherwise one error per colliding selection set, joined
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the recorded collisions are kept; calling again returns the same error
func (r *SourceGenerator) FieldNameCollisionError() error {
	return errors.Join(r.fieldNameCollisions...)
}

// checkGoNameCollision reports response keys of field selections that map to the same Go identifier.
//
// Arguments:
//   - selectionSet: the selections of one object
//   - typeName: the name of the struct the fields belong to, for the message
//
// Returns:
//   - error: nil if the Go field names of all field selections are distinct, otherwise an error
//     naming the struct, the colliding response keys, and the shared Go identifier
//
// Preconditions:
//   - none
//
// Postconditions:
//   - fragment spreads and inline fragments are not inspected; their own selection sets are
//     checked when they are built
func checkGoNameCollision(selectionSet ast.SelectionSet, typeName string) error {
	responseKeyByGoName := make(map[string]string, len(selectionSet))

	for _, selection := range selectionSet {
		field, ok := selection.(*ast.Field)
		if !ok {
			continue
		}

		goName := templates.ToGo(field.Alias)

		if previous, exists := responseKeyByGoName[goName]; exists && previous != field.Alias {
			return fmt.Errorf("%s: response keys %q and %q both become the Go field %s; alias one of them in the query", typeName, previous, field.Alias, goName)
		}

		responseKeyByGoName[goName] = field.Alias
	}

	return nil
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

// OperationArguments converts the variables of an operation into the arguments of its generated method.
//
// Arguments:
//   - variableDefinitions: the variables of the operation
//
// Returns:
//   - []*Argument: one argument per variable, in the order of variableDefinitions
//
// Preconditions:
//   - every variable type is bound to a Go type; otherwise it panics
//
// Postconditions:
//   - a variable with a default value has a Go type that can be nil, also when its GraphQL
//     type is non-null, because the caller may leave it to the server
//   - variableDefinitions itself is not modified
func (r *SourceGenerator) OperationArguments(variableDefinitions ast.VariableDefinitionList) []*Argument {
	argumentTypes := make([]*Argument, 0, len(variableDefinitions))
	for _, v := range variableDefinitions {
		variableType := v.Type
		if v.DefaultValue != nil && variableType.NonNull {
			nullableType := *variableType
			nullableType.NonNull = false
			variableType = &nullableType
		}

		argumentTypes = append(argumentTypes, &Argument{
			Variable: v.Variable,
			Type:     r.binder.CopyModifiersFromAst(variableType, r.Type(variableType.Name())),
		})
	}

	return argumentTypes
}

// Typeの引数に渡すtypeNameは解析した結果からselectionなどから求めた型の名前を渡さなければいけない
func (r *SourceGenerator) Type(typeName string) types.Type {
	model := r.cfg.Models[typeName].Model
	if len(model) == 0 {
		// Generate rejects this for a client-only config before plugins run.
		// The panic replaces the index panic for every other caller.
		panic(fmt.Sprintf("type %q is not bound to a Go type; add it to `autobind`/`models`, or set `model.filename` so gqlgenc generates it", typeName))
	}

	goType, err := r.binder.FindTypeFromName(model[0])
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
//   - a field with @skip or @include has the Go type and the tags of a nullable field
func (r *SourceGenerator) newFieldResponseField(selection *ast.Field, typeName string) *ResponseField {
	typeName = NewLayerTypeName(typeName, templates.ToGo(selection.Alias))
	fieldsResponseFields := r.NewResponseFields(selection.SelectionSet, typeName)

	baseType := r.fieldBaseType(selection, typeName, fieldsResponseFields)

	// A server leaves a field out of the response when @skip or @include says so,
	// also when the field is non-null, so the Go type must be able to say "absent".
	fieldType := selection.Definition.Type
	if fieldType.NonNull && isConditional(selection.Directives) {
		nullableType := *fieldType
		nullableType.NonNull = false
		fieldType = &nullableType
	}

	// return pointer type then optional type or slice pointer then slice type of definition in GraphQL.
	typ := r.binder.CopyModifiersFromAst(fieldType, baseType)

	isOptional := !fieldType.NonNull

	responseField := &ResponseField{
		Name:           selection.Alias,
		Type:           typ,
		Tags:           r.fieldTags(selection.Alias, isOptional),
		ResponseFields: fieldsResponseFields,
	}

	if !fieldsResponseFields.IsBasicType() {
		responseField.StructTypeName = typeName
		responseField.structFieldType = func(structType *types.Struct) types.Type {
			named := types.NewNamed(types.NewTypeName(0, r.client.Pkg(), typeName, nil), structType, nil)

			return r.binder.CopyModifiersFromAst(selection.Definition.Type, named)
		}
	}

	return responseField
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
//   - selection belongs to a validated query document
//
// Postconditions:
//   - an inline fragment without a type condition is treated as one on the enclosing type
//   - a struct type named after the type condition is appended to r.StructSources, unless the
//     selection set is a single fragment spread, whose type is reused instead
func (r *SourceGenerator) newInlineFragmentResponseField(selection *ast.InlineFragment, typeName string) *ResponseField {
	typeCondition := inlineFragmentTypeCondition(selection)

	// InlineFragment has child elements, so create a struct type here
	name := NewLayerTypeName(typeName, templates.ToGo(typeCondition))
	fieldsResponseFields := r.NewResponseFields(selection.SelectionSet, name)

	// if single fields that is also a fragment spread, reuse that fragment
	if len(fieldsResponseFields) == 1 && fieldsResponseFields[0].IsFragmentSpread {
		baseType := types.NewNamed(
			types.NewTypeName(0, r.client.Pkg(), templates.ToGo(fieldsResponseFields[0].Name), nil),
			fieldsResponseFields.StructType(),
			nil,
		)

		return &ResponseField{
			Name:           typeCondition,
			Type:           r.inlineFragmentType(baseType),
			Tags:           r.inlineFragmentTags(typeCondition),
			ResponseFields: fieldsResponseFields,
		}
	}

	fields := fieldsResponseFields
	if r.hasFragmentSpread(fieldsResponseFields) {
		fields = r.mergeInlineFragmentFields(fieldsResponseFields)
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
		Name:             typeCondition,
		Type:             r.inlineFragmentType(baseType),
		IsInlineFragment: true,
		Tags:             r.inlineFragmentTags(typeCondition),
		ResponseFields:   fields.SortByName(),
	}
}

// inlineFragmentTypeCondition returns the type an inline fragment applies to.
//
// Arguments:
//   - selection: the inline fragment
//
// Returns:
//   - string: the type condition, or the name of the enclosing type when the fragment has none
//
// Preconditions:
//   - selection belongs to a validated query document, so selection.ObjectDefinition is resolved
//
// Postconditions:
//   - the result is not empty
func inlineFragmentTypeCondition(selection *ast.InlineFragment) string {
	if selection.TypeCondition != "" {
		return selection.TypeCondition
	}

	return selection.ObjectDefinition.Name
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

// mergeInlineFragmentFields flattens the fragment spreads of an inline fragment into its own fields.
//
// Arguments:
//   - fieldsResponseFields: the fields of the inline fragment, including fragment spread entries
//
// Returns:
//   - ResponseFieldList: the own fields followed by the fields of the spreads, with a field selected both
//     directly and through a spread merged into one entry, in the order in which each field first appears
//
// Preconditions:
//   - fieldsResponseFields contains at least one fragment spread
//
// Postconditions:
//   - the struct sources of the merged spreads are recorded in r.StructSources
func (r *SourceGenerator) mergeInlineFragmentFields(fieldsResponseFields ResponseFieldList) ResponseFieldList {
	// collect all fields from fragment: the own fields first, then the fields of the spreads
	allFields := make(ResponseFieldList, 0, len(fieldsResponseFields))

	for _, field := range fieldsResponseFields {
		if !field.IsFragmentSpread {
			allFields = append(allFields, field)
		}
	}

	allFields = append(allFields, r.collectFragmentFields(fieldsResponseFields)...)

	// merge fields selected both directly and through a fragment, as a field selection does,
	// keeping the order in which they first appear
	generator := NewStructGenerator(fieldsResponseFields)
	r.StructSources = generator.MergedStructSources(r.StructSources)
	mergedFields := generator.GetCurrentResponseFieldList().MapByName()

	orderedFields := make(ResponseFieldList, 0, len(mergedFields))

	for _, field := range allFields {
		if merged, ok := mergedFields[field.Name]; ok {
			orderedFields = append(orderedFields, merged)
			delete(mergedFields, field.Name)
		}
	}

	return orderedFields
}

// inlineFragmentTags builds the struct tags of an inline fragment field.
//
// Arguments:
//   - typeCondition: the type the fragment applies to
//
// Returns:
//   - []string: the graphql tag "... on <typeCondition>", followed by the gqlgenc tag
//     "possibleTypes=<names>" that the decoder matches against __typename; the gqlgenc tag
//     is omitted when the schema has no object type satisfying typeCondition
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (r *SourceGenerator) inlineFragmentTags(typeCondition string) []string {
	graphqlTag := fmt.Sprintf(`graphql:"... on %s"`, typeCondition)

	possibleTypeNames := r.possibleTypeNames(typeCondition)
	if len(possibleTypeNames) == 0 {
		return []string{graphqlTag}
	}

	return []string{
		graphqlTag,
		fmt.Sprintf(`gqlgenc:"possibleTypes=%s"`, strings.Join(possibleTypeNames, ",")),
	}
}

// possibleTypeNames returns the object types that satisfy a type condition.
//
// Arguments:
//   - typeCondition: the name of an object, interface, or union type
//
// Returns:
//   - []string: the names sorted in ascending order: typeCondition itself for an object type,
//     the implementing object types for an interface, the member types for a union; nil when
//     the schema has no such type
//
// Preconditions:
//   - none
//
// Postconditions:
//   - interfaces that implement an interface are not included; __typename is always an object type
func (r *SourceGenerator) possibleTypeNames(typeCondition string) []string {
	definition, ok := r.cfg.Schema.Types[typeCondition]
	if !ok {
		return nil
	}

	possibleTypes := r.cfg.Schema.GetPossibleTypes(definition)
	possibleTypeNames := make([]string, 0, len(possibleTypes))

	for _, possibleType := range possibleTypes {
		if possibleType.Kind == ast.Object {
			possibleTypeNames = append(possibleTypeNames, possibleType.Name)
		}
	}

	slices.Sort(possibleTypeNames)

	return possibleTypeNames
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
