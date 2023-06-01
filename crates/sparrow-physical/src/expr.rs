use std::borrow::Cow;

use arrow_schema::DataType;

/// The identifier (index) of an expression.
#[derive(Debug, serde::Serialize, serde::Deserialize, PartialEq, Eq, PartialOrd, Ord, Hash)]
#[repr(transparent)]
#[serde(transparent)]
pub struct ExprId(usize);

impl From<usize> for ExprId {
    fn from(value: usize) -> Self {
        ExprId(value)
    }
}

/// A physical expression which describes how a value should be computed.
///
/// Generally, each expression computes a column of values from zero or more
/// input columns. Expressions appear in a variety of places within the steps
/// that make up a physical plan.
#[derive(Debug, serde::Serialize, serde::Deserialize)]

pub struct Expr {
    pub kind: ExprKind,
    /// Arguments to the expression.
    ///
    /// These are indices referencing earlier expressions.
    pub args: Vec<ExprId>,
    /// The type produced by the expression.
    pub result_type: DataType,
}

#[derive(
    Clone,
    Debug,
    Eq,
    Hash,
    Ord,
    PartialEq,
    PartialOrd,
    serde::Serialize,
    serde::Deserialize,
    enum_as_inner::EnumAsInner,
)]
#[serde(rename_all = "snake_case")]
pub enum ExprKind {
    /// Apply the named instruction to the given children.
    Call(Cow<'static, str>),
    /// Reference an input column by name.
    Column(String),
    /// A boolean literal.
    BooleanLiteral(bool),
    /// A string literal.
    StringLiteral(String),
    /// A numeric literal.
    ///
    /// Other primitive literals (such as date times) may be expressed
    /// using numeric literals with an appropriate datatype.
    NumericLiteral(bigdecimal::BigDecimal),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_physical_exprs_yaml() {
        let exprs = vec![
            Expr {
                kind: ExprKind::Column("foo".to_owned()),
                args: vec![],
                result_type: DataType::Int32,
            },
            Expr {
                kind: ExprKind::Column("bar".to_owned()),
                args: vec![],
                result_type: DataType::Int32,
            },
            Expr {
                kind: ExprKind::Call("add".into()),
                args: vec![0.into(), 1.into()],
                result_type: DataType::Int32,
            },
        ];

        // Use serde_yaml directly, since insta uses a non-standard Yaml formatter.
        let yaml = serde_yaml::to_string(&exprs).unwrap();
        insta::assert_snapshot!(yaml)
    }
}
