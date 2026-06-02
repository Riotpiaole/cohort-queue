# Elective Take-home

At Elective we bring course creators onto the platform in cohorts — fixed-size groups that move through onboarding together. Cohorts let us cap inflow into our Ops team so every creator gets the white-glove experience we're known for. Experience is everything here.

We're fortunate to have a large waiting list. We want a system to manage it: add creators in, pull cohorts out for onboarding, and always serve the people who have waited longest first (FIFO).

For simplicity, a creator is just the number `1`. Adding 10 means adding 10 creators.

---

## The Model

- A waiting list holds zero or more cohorts.
- A cohort has a fixed capacity (default 10) and can be partially filled.
- Cohorts are ordered: **newest on the left, oldest on the right**. We always serve (remove) from the right.
- Each number in the visualizations below is the current creator count of one cohort. So `[6, 10]` means two cohorts: a newer one with 6, an older one full at 10. The `10` is next to be served.

---

## Example Flow

| Action | State |
|---|---|
| Create a waiting list (capacity 10) | `[]` |
| Add 3 creators | `[3]` |
| Add 13 creators (7 fill the existing cohort; remaining 6 open a new cohort on the left) | `[6, 10]` |
| Add 22 creators (right cohort already full; the 6-cohort fills to 10; remaining 18 open two new cohorts) | `[8, 10, 10, 10]` |
| Take 4 (FIFO — pull from the right) | `[8, 10, 10, 6]` |
| Take 7 | `[8, 10, 9]` |
| Total waiting | `27` |
| Take 20 | `[7]` |
| Total waiting | `7` |

> Within a single cohort, the order of creators doesn't matter — only the ordering between cohorts.

---

## Features

The system should let us:

1. **Create** a waiting list with a configurable cohort capacity (default 10).
2. **Add** any number of creators in a single call. No cohort may exceed capacity.
3. **Take** up to N creators off the waiting list, oldest first.
4. **Get** the total number of creators currently waiting.

---

## Web Component

Wrap the system in a simple web page so we can interact with it — add, take, see current state. Look and feel are entirely up to you; we're not grading visual polish, just that it works and is sensible.

---

## On AI Tools

We use AI heavily at Elective and expect you will too. Use whatever helps — Claude, Codex, Cursor, ChatGPT — no need to hide it.

What we care about is **how you use it**. In your writeup, include a short section on:

- Where AI helped, and where you overrode it (or had to).
- One moment where the AI suggestion was wrong or sloppy, and what you did instead.
- Anything you decided to write by hand and why.

A strong submission isn't "no AI used" — it's a candidate who clearly drove the work and can defend every line.

---

## Edge Cases & Considerations

Be prepared to walk us through:

- The edge cases you identified and how you handled them (take 0, take more than total, add 0, capacity of 1, empty cohorts after removal, etc.).
- The business-logic decisions you made when the spec was silent.
- Anything you considered for performance, structure, or future change.

---

## Time Budget

We expect this to take **4–6 hours**. If you find yourself going much over, stop and write up what you'd have done with more time. We'd rather see a clean partial than a kitchen sink.

---

## What We're Grading On

| Area | What matters |
|---|---|
| **TypeScript judgment** | Does the type system prevent bugs, or is it decoration? |
| **Edge-case handling** | What did you notice, and what did you do about it? |
| **Code structure & communication** | Clean code is table stakes; explaining *why* you chose it matters more. |
| **AI collaboration** | How you drove the tools, not whether you used them. |

---

## Submission

Email your solution to [developers@elective.com](mailto:developers@elective.com) with subject line:

```
Cohort waiting list: ${yourName}
```

Include a link to your repository and your writeup (a README section is fine).

Questions before you start? Shoot us a message — we'll get back to you quickly.

**Happy coding!**