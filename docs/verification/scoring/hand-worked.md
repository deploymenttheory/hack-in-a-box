# Independent scoring fixture

Issue #114 uses synthetic inputs and fixed expected answers stored in
`internal/app/testdata/scoring114-independent.json`. No production calculator or
rounding helper generates the expected answers. The native test writes actual
independent judge cards and reads the actual result owner in both scoring modes.

| Category | Weight | Judge A | Judge B | Judge C | Exact mean | Weighted mean |
|---|---:|---:|---:|---:|---|---|
| Demo | 2 | 9 | 4 | 7 | 20/3 | 40/3 |
| Technical approach | 3 | 2 | 8 | 7 | 17/3 | 51/3 |
| UX | 4 | 7 | 3 | 8 | 18/3 = 6 | 72/3 |
| Feature fit | 1 | 4 | 9 | 2 | 15/3 = 5 | 15/3 |
| Innovation | 4 | 8 | 6 | 5 | 19/3 | 76/3 |

The final total is `(40 + 51 + 72 + 15 + 76) / 3 = 254/3`.
Its displayed value is **84.67**. The stored expected numerator and denominator
remain 254 and 3; display text is never the arithmetic input.

The two live provisional checkpoints use 9 and 10 in every category. Their
weighted totals would be 126 and 140, but neither is accumulated. The subsequent
signed mentor final uses 3 in every category: `3 × (2+3+4+1+4) = 42`.
Qualification retains **42**, never 308; the fresh final retains **254/3**,
never `254/3 + 42` or `254/3 + 126 + 140 + 42`.

The final remains unavailable when any required judge is absent or has only a
saved pending card. The native denominator must remain three. Completing the
third independent card does not authorise confirmation: opening deliberation
and the separately reviewed result actions remain necessary.

Other hand-worked exact/near-tie fixtures remain in
`internal/results/calculation_test.go`: 246/49 and 251/50 both display 5.02,
yet the former is strictly larger because `246×50 = 12300 > 12299 = 251×49`.
Exact equal totals retain equal ranks; UUID order cannot decide a winner.
