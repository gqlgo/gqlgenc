package introspection

import (
	"errors"
	"fmt"
)

// errInvalidIntrospection は、introspection の応答が GraphQL の仕様から外れていて、スキーマを
// 組み立てられないことを表す。
var errInvalidIntrospection = errors.New("invalid introspection response")

// validateIntrospection は、スキーマを組み立てる前に、組み立てを続けられない応答をエラーにする。
// 型の一覧の null の要素・名前の無い型・名前付きの型の種類でない kind・型の一覧に無い型を指す
// queryType / mutationType / subscriptionType と型の参照・null のフィールドや引数などを見る。
// これらをそのまま組み立てると、nil ポインタの参照か、型の種類が分からないことによる panic になる。
//
// 型どうしの関係 (interface を実装しているかなど) は、組み立てた後の validator.ValidateSchemaDocument
// が確かめるため、ここでは見ない。
//
// 事後条件: nil を返したとき、parser はこの応答を panic せずに組み立てられる (型の入れ子が
// introspection クエリの深さを超えるときの errIntrospectionTypeTooDeep を除く)。
func validateIntrospection(query Query) error {
	typeMap := make(map[string]*FullType, len(query.Schema.Types))
	for i, typ := range query.Schema.Types {
		if typ == nil {
			return fmt.Errorf("%w: types[%d] is null", errInvalidIntrospection, i)
		}
		if typ.Name == nil {
			return fmt.Errorf("%w: types[%d] has no name", errInvalidIntrospection, i)
		}
		switch typ.Kind {
		case TypeKindScalar, TypeKindObject, TypeKindInterface, TypeKindUnion, TypeKindEnum, TypeKindInputObject:
		case TypeKindList, TypeKindNonNull:
			return fmt.Errorf("%w: type %q has kind %s, which is not a named type kind", errInvalidIntrospection, *typ.Name, typ.Kind)
		default:
			return fmt.Errorf("%w: type %q has unknown kind %q", errInvalidIntrospection, *typ.Name, typ.Kind)
		}
		typeMap[*typ.Name] = typ
	}

	if err := validateOperationTypes(query, typeMap); err != nil {
		return err
	}

	for _, typ := range query.Schema.Types {
		if err := validateType(typ, typeMap); err != nil {
			return err
		}
	}

	for i, directive := range query.Schema.Directives {
		if directive == nil {
			return fmt.Errorf("%w: directives[%d] is null", errInvalidIntrospection, i)
		}
		if err := validateInputValues(directive.Args, typeMap, "argument of directive @"+directive.Name); err != nil {
			return err
		}
	}

	return nil
}

// validateOperationTypes は、queryType・mutationType・subscriptionType が型の一覧にある型を指すかを
// 確かめる。queryType の名前が無い応答は、これまでどおり Query の操作の型を持たないスキーマにする。
func validateOperationTypes(query Query, typeMap map[string]*FullType) error {
	if name := query.Schema.QueryType.Name; name != nil {
		if err := validateOperationType("queryType", name, typeMap); err != nil {
			return err
		}
	}
	if mutationType := query.Schema.MutationType; mutationType != nil {
		if err := validateOperationType("mutationType", mutationType.Name, typeMap); err != nil {
			return err
		}
	}
	if subscriptionType := query.Schema.SubscriptionType; subscriptionType != nil {
		if err := validateOperationType("subscriptionType", subscriptionType.Name, typeMap); err != nil {
			return err
		}
	}

	return nil
}

// validateOperationType は、操作の型 operation (queryType など) が名前を持ち、型の一覧にある型を
// 指すかを確かめる。
func validateOperationType(operation string, name *string, typeMap map[string]*FullType) error {
	if name == nil {
		return fmt.Errorf("%w: %s has no name", errInvalidIntrospection, operation)
	}
	if _, ok := typeMap[*name]; !ok {
		return fmt.Errorf("%w: %s %q is not in types", errInvalidIntrospection, operation, *name)
	}

	return nil
}

// validateType は、型のフィールド・引数・input フィールド・interface・enum の値・union の型に
// null の要素が無く、フィールドと引数の型が型の一覧にある型を指すかを確かめる。
func validateType(typ *FullType, typeMap map[string]*FullType) error {
	name := *typ.Name

	for i, field := range typ.Fields {
		if field == nil {
			return fmt.Errorf("%w: fields[%d] of type %q is null", errInvalidIntrospection, i, name)
		}
		owner := fmt.Sprintf("field %s.%s", name, field.Name)
		if err := validateTypeRef(field.Type, typeMap, owner); err != nil {
			return err
		}
		if err := validateInputValues(field.Args, typeMap, "argument of "+owner); err != nil {
			return err
		}
	}

	if err := validateInputValues(typ.InputFields, typeMap, "input field of type "+name); err != nil {
		return err
	}

	for i, intf := range typ.Interfaces {
		if intf == nil {
			return fmt.Errorf("%w: interfaces[%d] of type %q is null", errInvalidIntrospection, i, name)
		}
	}

	for i, enumValue := range typ.EnumValues {
		if enumValue == nil {
			return fmt.Errorf("%w: enumValues[%d] of type %q is null", errInvalidIntrospection, i, name)
		}
	}

	for i, possibleType := range typ.PossibleTypes {
		if possibleType == nil || possibleType.Name == nil {
			return fmt.Errorf("%w: possibleTypes[%d] of type %q has no name", errInvalidIntrospection, i, name)
		}
	}

	return nil
}

// validateInputValues は、引数か input フィールドの一覧に null の要素が無く、それぞれの型が型の一覧に
// ある型を指すかを確かめる。owner はエラーに載せる、その一覧を持つ要素の説明。
func validateInputValues(inputValues []*InputValue, typeMap map[string]*FullType, owner string) error {
	for i, inputValue := range inputValues {
		if inputValue == nil {
			return fmt.Errorf("%w: %s at index %d is null", errInvalidIntrospection, owner, i)
		}
		if err := validateTypeRef(inputValue.Type, typeMap, owner+" "+inputValue.Name); err != nil {
			return err
		}
	}

	return nil
}

// validateTypeRef は、型の参照の LIST・NON_NULL を外した名前付きの型が、名前を持ち、型の一覧に
// あるかを確かめる。ofType が途中で無い参照は、組み立てるときの errIntrospectionTypeTooDeep に任せる。
func validateTypeRef(ref TypeRef, typeMap map[string]*FullType, owner string) error {
	for ref.Kind == TypeKindList || ref.Kind == TypeKindNonNull {
		if ref.OfType == nil {
			return nil
		}
		ref = *ref.OfType
	}

	if ref.Name == nil {
		return fmt.Errorf("%w: the type of %s has no name", errInvalidIntrospection, owner)
	}
	if _, ok := typeMap[*ref.Name]; !ok {
		return fmt.Errorf("%w: the type %q of %s is not in types", errInvalidIntrospection, *ref.Name, owner)
	}

	return nil
}
