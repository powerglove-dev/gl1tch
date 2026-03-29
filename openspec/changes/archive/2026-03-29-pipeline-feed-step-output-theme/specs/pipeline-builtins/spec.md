## MODIFIED Requirements

### Requirement: builtin.assert validates a condition
`builtin.assert` SHALL evaluate the `condition` arg as a boolean expression and return an error if it is false. Supported expressions: `"true"`, `"false"`, `"not_empty"` (value is non-empty after trimming whitespace), `"contains:<str>"` against `value` arg, `"matches:<regex>"` against `value` arg, `"len > <n>"` against `value` arg. On failure, the error message SHALL include the `message` arg if provided.

#### Scenario: Assert passes on true condition
- **WHEN** `builtin.assert` is called with `condition: "true"`
- **THEN** the step succeeds with no output

#### Scenario: Assert fails on false condition
- **WHEN** `builtin.assert` is called with `condition: "false"` and `message: "expected pass"`
- **THEN** the step returns an error containing `"expected pass"`

#### Scenario: Assert contains check
- **WHEN** `builtin.assert` is called with `condition: "contains:hello"` and `value: "hello world"`
- **THEN** the step succeeds

#### Scenario: Assert not_empty passes when value has content
- **WHEN** `builtin.assert` is called with `condition: "not_empty"` and `value: "some text"`
- **THEN** the step succeeds with output `{"passed": true}`

#### Scenario: Assert not_empty fails when value is empty
- **WHEN** `builtin.assert` is called with `condition: "not_empty"` and `value: ""`
- **THEN** the step returns an error

#### Scenario: Assert not_empty fails when value is whitespace only
- **WHEN** `builtin.assert` is called with `condition: "not_empty"` and `value: "   "`
- **THEN** the step returns an error
