## ADDED Requirements

### Requirement: Bound recursive syntax and semantic trees
Gregor SHALL limit active statement/factor parser entries to 256 and paths of statement, condition and expression AST nodes to 256. Containers do not add AST depth. It SHALL check trees iteratively before recursive examination or generation, including manually constructed trees. Rejections SHALL retain source identity and position.

#### Scenario: Exact supported boundary
- **WHEN** syntax and AST depth are within both budgets
- **THEN** ordinary compilation succeeds as otherwise specified

#### Scenario: Excessive recursion or left association
- **WHEN** parentheses, prefix factors, nested statements, conditions or left-associated expressions exceed either budget
- **THEN** compilation rejects safely without recursive traversal beyond the budget

#### Scenario: Imported or manual tree
- **WHEN** an over-depth tree belongs to an imported declaration or is supplied directly to compilation
- **THEN** its rejection retains its owning source and a useful position
