use arrow_schema::DataType;

use crate::evaluator::Evaluator;
use crate::values::StructValue;
use crate::Error;

inventory::submit!(crate::evaluators::EvaluatorFactory {
    name: "field_ref",
    create: &create
});

/// Evaluator for `field_ref` expressions.
struct FieldRefEvaluator {
    input: StructValue,
    field: usize,
}

impl Evaluator for FieldRefEvaluator {
    fn evaluate(
        &self,
        work_area: &crate::work_area::WorkArea<'_>,
    ) -> error_stack::Result<arrow_array::ArrayRef, crate::Error> {
        let input = work_area.expression(self.input);
        let field = input.column(self.field).clone();
        Ok(field)
    }
}

fn create(info: super::StaticInfo<'_>) -> error_stack::Result<Box<dyn Evaluator>, crate::Error> {
    let field = info.literal_string()?;
    let result_type = info.result_type;

    let name = info.name;
    let input = info.unpack_argument()?;

    let field = match input.data_type {
        DataType::Struct(fields) => {
            let (index, field) = fields.find(field).ok_or_else(|| Error::NoSuchField {
                field_name: field.to_owned(),
            })?;
            error_stack::ensure!(
                result_type == field.data_type(),
                Error::UnexpectedResultType {
                    name: name.clone(),
                    actual: field.data_type().clone(),
                    expected: result_type.clone()
                }
            );
            index
        }
        actual => {
            error_stack::bail!(Error::InvalidNonStructArgumentType {
                actual: actual.clone()
            })
        }
    };

    Ok(Box::new(FieldRefEvaluator {
        input: input.struct_()?,
        field,
    }))
}
