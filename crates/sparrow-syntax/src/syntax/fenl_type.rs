use std::str::FromStr;
use std::{fmt::Display, sync::Arc};

use arrow_schema::{DataType, Field, FieldRef, Fields, IntervalUnit, TimeUnit};
use itertools::Itertools;
use serde::Serialize;
use sparrow_arrow::scalar_value::timeunit_suffix;

use crate::TypeVariable;

/// A wrapper around an Arrow `DataType`.
///
/// Adapts / extends the underlying Arrow type with additional Fenl-specific
/// types.
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
#[cfg_attr(test, derive(serde::Serialize))]
pub enum FenlType {
    // TODO: Rename concrete?
    /// A specific Arrow DataType.
    Concrete(DataType),
    /// A generic type with the given type variable.
    TypeRef(TypeVariable),
    /// A collection type with the given type variable(s).
    ///
    /// e.g. (Collection::Map, [TypeVariable("K"), TypeVariable("V")])
    ///
    /// TODO(https://github.com/kaskada-ai/kaskada/issues/494): Support FenlType
    Collection(Collection, Vec<TypeVariable>),
    /// A type for describing a windowing behavior.
    Window,
    /// A type for describing a string that will be interpreted
    /// as a json object.
    Json,
    /// Added to indicate that a type is the result of an invalid expression.
    ///
    /// This indicates the error has already been reported, so no more error
    /// reports are needed.
    Error,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash)]
#[cfg_attr(test, derive(serde::Serialize))]
pub enum Collection {
    List,
    Map,
}

impl std::fmt::Display for Collection {
    fn fmt(&self, fmt: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Collection::List => fmt.write_str("list"),
            Collection::Map => fmt.write_str("map"),
        }
    }
}

/// A wrapper for formatting DataTypes.
pub struct FormatDataType<'a>(pub &'a DataType);

impl<'a> std::fmt::Display for FormatDataType<'a> {
    fn fmt(&self, fmt: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self.0 {
            DataType::Null => fmt.write_str("null"),
            DataType::Boolean => fmt.write_str("bool"),
            DataType::Utf8 => fmt.write_str("string"),
            DataType::LargeUtf8 => fmt.write_str("large_string"),
            DataType::Int32 => fmt.write_str("i32"),
            DataType::Int64 => fmt.write_str("i64"),
            DataType::UInt32 => fmt.write_str("u32"),
            DataType::UInt64 => fmt.write_str("u64"),
            DataType::Float32 => fmt.write_str("f32"),
            DataType::Float64 => fmt.write_str("f64"),
            DataType::Interval(IntervalUnit::DayTime) => fmt.write_str("interval_days"),
            DataType::Interval(IntervalUnit::YearMonth) => fmt.write_str("interval_months"),
            DataType::Duration(timeunit) => {
                write!(fmt, "duration_{}", timeunit_suffix(timeunit))
            }
            DataType::Timestamp(timeunit, None) => {
                write!(fmt, "timestamp_{}", timeunit_suffix(timeunit))
            }
            DataType::Struct(fields) => {
                write!(fmt, "{}", FormatStruct(fields))
            }
            DataType::Date32 => fmt.write_str("date32"),
            DataType::Map(f, _) => match f.data_type() {
                DataType::Struct(fields) => {
                    write!(
                        fmt,
                        "map<{}, {}>",
                        FormatDataType(fields[0].data_type()),
                        FormatDataType(fields[1].data_type()),
                    )
                }
                other => panic!("expected struct, saw {:?}", other),
            },
            _ => unimplemented!("Display for type {:?}", self.0),
        }
    }
}

// Creates a struct that can be given a reference to set of fields
/// A wrapper for formatting structs.
pub struct FormatStruct<'a>(pub &'a [FieldRef]);
impl<'a> std::fmt::Display for FormatStruct<'a> {
    fn fmt(&self, fmt: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            fmt,
            "{{{}}}",
            self.0.iter().format_with(", ", |field, f| {
                f(&format_args!(
                    "{}: {}",
                    field.name(),
                    FenlType::Concrete(field.data_type().clone())
                ))
            })
        )
    }
}

/// Fenl uses a limited form of ad-hoc polymorphism for functions.
///
/// Specifically, function signatures may have a single type variable
/// constrained in one of the following ways.
///
/// All occurrences of the type variable must be the same type within
/// an instantiation of the signature. This leads to a relatively simple
/// instantiation strategy, where a least upper bound of the types of the
/// actual arguments is chosen for constrained type.
#[derive(PartialOrd, Ord, Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize)]
#[repr(u8)]
pub enum TypeClass {
    /// Any type.
    Any,
    /// Any type that is a valid key.
    Key,
    /// Any numeric type.
    Number,
    /// Any signed numeric type.
    Signed,
    /// Any floating point numeric type.
    Float,
    /// Any time delta.
    TimeDelta,
    /// Any ordered type. This includes numbers and timestamps.
    Ordered,
    /// Error variant.
    ///
    /// This indicates the error has already been reported, so no more error
    /// reports are needed.
    Error,
}

impl Display for TypeClass {
    fn fmt(&self, fmt: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            TypeClass::Number => fmt.write_str("number"),
            TypeClass::Key => fmt.write_str("key"),
            TypeClass::Any => fmt.write_str("any"),
            TypeClass::Signed => fmt.write_str("signed"),
            TypeClass::Float => fmt.write_str("float"),
            TypeClass::TimeDelta => fmt.write_str("timedelta"),
            TypeClass::Ordered => fmt.write_str("ordered"),
            TypeClass::Error => fmt.write_str("error"),
        }
    }
}

impl FromStr for TypeClass {
    type Err = TypeClass;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "number" => Ok(TypeClass::Number),
            "key" => Ok(TypeClass::Key),
            "any" => Ok(TypeClass::Any),
            "signed" => Ok(TypeClass::Signed),
            "float" => Ok(TypeClass::Float),
            "timedelta" => Ok(TypeClass::TimeDelta),
            "ordered" => Ok(TypeClass::Ordered),
            _ => Err(TypeClass::Error),
        }
    }
}

/// Concrete windowing behavior describes how the given window will affect the
/// aggregation.
#[derive(Clone, Copy, Serialize, Debug, PartialEq, Hash, Eq, Ord, PartialOrd)]
pub enum WindowBehavior {
    Since,
    Sliding,
}

impl WindowBehavior {
    pub fn label(&self) -> &'static str {
        match self {
            Self::Since => "since",
            Self::Sliding => "sliding",
        }
    }
}

impl Display for FenlType {
    fn fmt(&self, fmt: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            FenlType::Json => write!(fmt, "json"),
            FenlType::Window => write!(fmt, "window"),
            FenlType::TypeRef(type_param) => write!(fmt, "{type_param}"),
            FenlType::Concrete(data_type) => write!(fmt, "{}", FormatDataType(data_type)),
            FenlType::Error => write!(fmt, "error"),
            FenlType::Collection(c, vars) => {
                write!(fmt, "{}<{}>", c, vars.iter().format(", "))
            }
        }
    }
}

impl FromStr for FenlType {
    type Err = FenlType;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "bool" => Ok(DataType::Boolean.into()),
            "i8" => Ok(DataType::Int8.into()),
            "i32" => Ok(DataType::Int32.into()),
            "i64" => Ok(DataType::Int64.into()),
            "u8" => Ok(DataType::UInt8.into()),
            "u16" => Ok(DataType::UInt16.into()),
            "u32" => Ok(DataType::UInt32.into()),
            "u64" => Ok(DataType::UInt64.into()),
            "f32" => Ok(DataType::Float32.into()),
            "f64" => Ok(DataType::Float64.into()),
            "string" => Ok(DataType::Utf8.into()),
            "large_string" => Ok(DataType::LargeUtf8.into()),
            "interval_days" => Ok(DataType::Interval(IntervalUnit::DayTime).into()),
            "interval_months" => Ok(DataType::Interval(IntervalUnit::YearMonth).into()),
            "timestamp_s" => Ok(DataType::Timestamp(TimeUnit::Second, None).into()),
            "timestamp_ms" => Ok(DataType::Timestamp(TimeUnit::Millisecond, None).into()),
            "timestamp_us" => Ok(DataType::Timestamp(TimeUnit::Microsecond, None).into()),
            "timestamp_ns" => Ok(DataType::Timestamp(TimeUnit::Nanosecond, None).into()),
            "duration_s" => Ok(DataType::Duration(TimeUnit::Second).into()),
            "duration_ms" => Ok(DataType::Duration(TimeUnit::Millisecond).into()),
            "duration_us" => Ok(DataType::Duration(TimeUnit::Microsecond).into()),
            "duration_ns" => Ok(DataType::Duration(TimeUnit::Nanosecond).into()),
            "window" => Ok(FenlType::Window),
            "json" => Ok(FenlType::Json),
            // TODO(https://github.com/kaskada-ai/kaskada/issues/494): Support fenl types
            // in collections
            s if s.starts_with("list<") && s.ends_with('>') => {
                let type_var = &s[5..s.len() - 1]
                    .split(',')
                    .map(|s| s.trim())
                    .collect::<Vec<_>>();

                // One type var for a list
                if type_var.len() != 1 {
                    return Err(FenlType::Error);
                }

                match FenlType::from_str(type_var[0])? {
                    FenlType::Concrete(dt) => {
                        let f = Field::new("item", dt, true);
                        Ok(DataType::List(Arc::new(f)).into())
                    }
                    FenlType::TypeRef(type_var) => {
                        Ok(FenlType::Collection(Collection::List, vec![type_var]))
                    }
                    other => panic!("unexpected type: {:?}", other),
                }
            }
            s if s.starts_with("map<") && s.ends_with('>') => {
                let type_var = &s[4..s.len() - 1]
                    .split(',')
                    .map(|s| s.trim())
                    .collect::<Vec<_>>();

                // Two type vars for a map
                if type_var.len() != 2 {
                    return Err(FenlType::Error);
                }
                let key_type = FenlType::from_str(type_var[0])?;
                let value_type = FenlType::from_str(type_var[1])?;

                match (key_type, value_type) {
                    (FenlType::Concrete(kt), FenlType::Concrete(vt)) => {
                        let f1 = Field::new("key", kt, true);
                        let f2 = Field::new("value", vt, true);
                        let s = DataType::Struct(Fields::from(vec![f1, f2]));
                        let f = Field::new("entries", s, true);
                        Ok(DataType::Map(Arc::new(f), false).into())
                    }
                    (FenlType::TypeRef(ktv), FenlType::TypeRef(vtv)) => {
                        Ok(FenlType::Collection(Collection::Map, vec![ktv, vtv]))
                    }
                    (_, _) => unimplemented!("map with concrete and type variable mix"),
                }
            }
            s => Ok(FenlType::TypeRef(TypeVariable(s.to_owned()))),
        }
    }
}

impl From<DataType> for FenlType {
    fn from(data_type: DataType) -> Self {
        Self::Concrete(data_type)
    }
}

impl From<&Field> for FenlType {
    fn from(field: &Field) -> Self {
        Self::Concrete(field.data_type().clone())
    }
}

impl FenlType {
    pub fn is_error(&self) -> bool {
        matches!(self, FenlType::Error)
    }

    pub fn arrow_type(&self) -> Option<&DataType> {
        match self {
            FenlType::Collection(_, _) => None,
            FenlType::TypeRef(_) => None,
            FenlType::Concrete(t) => Some(t),
            FenlType::Window => None,
            FenlType::Json => None,
            FenlType::Error => None,
        }
    }

    pub fn record_fields(&self) -> Option<&Fields> {
        self.arrow_type().and_then(|data_type| match data_type {
            DataType::Struct(fields) => Some(fields),
            _ => None,
        })
    }

    pub fn take_arrow_type(self) -> Option<DataType> {
        match self {
            FenlType::Collection(_, _) => None,
            FenlType::TypeRef(_) => None,
            FenlType::Concrete(t) => Some(t),
            FenlType::Window => None,
            FenlType::Json => None,
            FenlType::Error => None,
        }
    }
}
