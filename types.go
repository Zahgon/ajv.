package ajv

// Logger receives the messages Ajv sends to its logger. Each method gets the same arguments as
// the corresponding JavaScript logger method.
type Logger interface {
	Log(args ...any)
	Warn(args ...any)
	Error(args ...any)
}

// Options are the instance options. Each field corresponds to the Ajv option of the same name
// (Data is $data). A field of type any is unset when nil and otherwise holds the JavaScript value
// of the option (a bool, a string or a number). LoadSchema nil is unset, and Logger nil is the
// default logger, which writes to standard error.
type Options struct {
	Strict                  any
	StrictSchema            any
	StrictNumbers           any
	StrictTypes             any
	StrictTuples            any
	StrictRequired          any
	AllowMatchingProperties bool
	AllowUnionTypes         bool
	ValidateFormats         any
	Data                    bool
	AllErrors               bool
	Verbose                 bool
	Discriminator           bool
	UnicodeRegExp           any
	Messages                any
	OwnProperties           bool
	MultipleOfPrecision     int
	CoerceTypes             any
	UseDefaults             any
	RemoveAdditional        any
	InlineRefs              any
	ValidateSchema          any
	AddUsedSchema           any
	LoadSchema              func(uri string) (any, error)
	Logger                  Logger
}

// ErrorsTextOptions are the options of ErrorsText. Empty fields take the defaults ", " and
// "data".
type ErrorsTextOptions struct {
	Separator string
	DataVar   string
}

// ErrorObject is a validation error. Params and PropertyName are nil when absent and Message is
// empty when absent. Schema, ParentSchema and Data are set in verbose mode.
type ErrorObject struct {
	Keyword      string
	InstancePath string
	SchemaPath   string
	Params       *Object
	PropertyName *string
	Message      string
	Schema       any
	ParentSchema any
	Data         any
}

// MissingRefError is the error for a reference that cannot be resolved.
type MissingRefError struct {
	Message       string
	MissingRef    string
	MissingSchema string
}

// Error returns the message.
func (e *MissingRefError) Error() string { return e.Message }

// DataValidationCxt is the data context a keyword function receives: the JSON Pointer of the
// data, the object or array holding it and its key or index there (nil at the root), and the
// data passed to the top-level validation function.
type DataValidationCxt struct {
	InstancePath       string
	ParentData         any
	ParentDataProperty any
	RootData           any
}

// KeywordDefinition defines a keyword for AddKeyword. The fields mirror Ajv's keyword
// definition: Data is $data; Type and SchemaType are nil, a string or a []string; Errors is nil,
// a bool or "full"; Valid is nil or a bool.
//
// Validate returns the validation result and the errors: nil errors leave the function's errors
// null, a non-nil slice (even an empty one) sets them. Compile returns the validation function;
// a nil function with a nil error means the compilation produced undefined, and an error is
// thrown. Macro returns the schema the keyword expands to (nil is undefined). An error a keyword
// function returns with an empty InstancePath has its instance path filled in by Ajv.
type KeywordDefinition struct {
	Keyword        string
	Type           any
	SchemaType     any
	Data           bool
	Validate       func(schema, data, parentSchema any, cxt DataValidationCxt) (bool, []*ErrorObject)
	Compile        func(schema, parentSchema any) (func(data any, cxt DataValidationCxt) (bool, []*ErrorObject), error)
	Macro          func(schema, parentSchema any) any
	Errors         any
	MetaSchema     any
	Dependencies   []string
	Modifying      bool
	Valid          any
	Before         string
	Post           bool
	Implements     []string
	AllowUndefined bool
}
