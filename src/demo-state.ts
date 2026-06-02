/**
 * Scripted static demo state.
 *
 * This is NOT the cohort engine. It is a hand-authored "tour": a fixed list of
 * snapshots that tells one coherent story — grow the list (adds open cohorts on
 * the LEFT, past 20 to show the middle-collapse), then drain it (takes remove
 * from the RIGHT, oldest first). A single cursor walks this tour:
 *
 *   - create -> jump to the empty state (index 0)
 *   - add    -> step toward the most-grown ("peak") snapshot
 *   - take   -> step from peak toward empty, draining the right
 *
 * The request body's `n` is intentionally ignored here: the snapshots are canned.
 * The real, N-aware, capacity-enforcing engine is the explicit next step.
 */

import type { WaitingListState } from "./types.js";

const DEMO_CAPACITY = 10;

/** Build `k` full cohorts (each at capacity) — keeps the big snapshots terse. */
const fulls = (k: number): number[] => Array.from({ length: k }, () => DEMO_CAPACITY);

interface TourStep {
  readonly op: "create" | "add" | "take";
  readonly cohorts: readonly number[];
}

/**
 * The tour. Index 0 is empty; the list grows to PEAK_INDEX (a 25-cohort state),
 * then drains back to empty. Each transition is locally correct: adds change the
 * LEFT, takes shrink from the RIGHT.
 */
const TOUR: readonly TourStep[] = [
  { op: "create", cohorts: [] }, //                            0  []
  { op: "add", cohorts: [3] }, //                              1  matches PROBLEMS.md
  { op: "add", cohorts: [6, 10] }, //                          2  matches PROBLEMS.md
  { op: "add", cohorts: [8, 10, 10, 10] }, //                  3  matches PROBLEMS.md
  { op: "add", cohorts: [5, ...fulls(15)] }, //                4  16 cohorts
  { op: "add", cohorts: [9, ...fulls(24)] }, //                5  25 cohorts  <- PEAK (collapse)
  { op: "take", cohorts: [9, ...fulls(23), 6] }, //            6  drained 4 from oldest (10->6)
  { op: "take", cohorts: [9, ...fulls(23)] }, //               7  oldest cohort emptied & removed (24)
  { op: "take", cohorts: [9, ...fulls(13)] }, //               8  drained more oldest (14)
  { op: "take", cohorts: [9, 10, 4] }, //                      9  3 cohorts
  { op: "take", cohorts: [7] }, //                            10  1 cohort
  { op: "take", cohorts: [] }, //                             11  empty
];

const PEAK_INDEX = 5;
const LAST_INDEX = TOUR.length - 1;

/** Module-level cursor. Single-user demo: a shared cursor is fine (see LOG.md). */
let cursor = 0;
let capacity = DEMO_CAPACITY;

const clamp = (i: number): number => Math.min(Math.max(i, 0), LAST_INDEX);

const snapshot = (): WaitingListState => ({
  capacity,
  cohorts: TOUR[clamp(cursor)]!.cohorts,
});

/** Reset the tour to empty. `capacity` is accepted for realism but display-only here. */
export function create(newCapacity: number = DEMO_CAPACITY): WaitingListState {
  capacity = newCapacity;
  cursor = 0;
  return snapshot();
}

/** Step toward the most-grown snapshot (adds on the left). No-op once at peak. */
export function add(): WaitingListState {
  cursor = cursor < PEAK_INDEX ? cursor + 1 : PEAK_INDEX;
  return snapshot();
}

/** Step toward empty, draining the right. From the grow phase, jump to peak first. */
export function take(): WaitingListState {
  cursor = cursor < PEAK_INDEX ? PEAK_INDEX : clamp(cursor + 1);
  return snapshot();
}

/** Current snapshot without mutating the cursor. */
export function current(): WaitingListState {
  return snapshot();
}

/** Total creators currently waiting across all cohorts. */
export function total(state: WaitingListState = snapshot()): number {
  return state.cohorts.reduce((sum, c) => sum + c, 0);
}
