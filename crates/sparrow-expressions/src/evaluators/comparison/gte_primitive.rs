use std::sync::Arc;

use arrow_array::{ArrayRef, ArrowNumericType};
use error_stack::{IntoReport, ResultExt};

use crate::evaluator::Evaluator;
use crate::evaluators::StaticInfo;
use crate::values::PrimitiveValue;
use crate::work_area::WorkArea;
use crate::Error;

inventory::submit!(crate::evaluators::EvaluatorFactory {
    name: "gte_primitive",
    create: &crate::evaluators::macros::create_primitive_evaluator!(0, create, ordered)
});

/// Evaluator for the `gte_primitive` (greater than or equal) instruction.
struct GtePrimitiveEvaluator<T: ArrowNumericType> {
    lhs: PrimitiveValue<T>,
    rhs: PrimitiveValue<T>,
}

impl<T: ArrowNumericType> Evaluator for GtePrimitiveEvaluator<T> {
    fn evaluate(&self, work_area: &WorkArea<'_>) -> error_stack::Result<ArrayRef, Error> {
        let lhs = work_area.expression(self.lhs);
        let rhs = work_area.expression(self.rhs);

        let result = arrow_ord::comparison::gt_eq::<T>(lhs, rhs)
            .into_report()
            .change_context(Error::ExprEvaluation)?;
        Ok(Arc::new(result))
    }
}

fn create<T: ArrowNumericType>(
    info: StaticInfo<'_>,
) -> error_stack::Result<Box<dyn Evaluator>, Error> {
    let (lhs, rhs) = info.unpack_arguments()?;

    // We don't need to check the data type -- the `lhs.primitive()` and `rhs.primitive()` will
    // confirm that is compatible with the primitive type.
    Ok(Box::new(GtePrimitiveEvaluator::<T> {
        lhs: lhs.primitive()?,
        rhs: rhs.primitive()?,
    }))
}
