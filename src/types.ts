/**
 * The typed contract shared between the API and the visualization.
 *
 * NOTE: this is the *static demo*. These types document the invariants the real
 * WaitingList engine will guarantee, but the demo serves hand-authored snapshots
 * and does not enforce them at runtime (no engine yet — see LOG.md / README).
 */

/**
 * A single cohort's current creator count.
 * Invariant (to be enforced by the future engine): 1 <= count <= capacity.
 * A cohort never persists at 0 — it is removed once fully drained.
 */
export type CohortCount = number;

/**
 * A snapshot of the waiting list.
 *
 * Cohorts are ordered newest -> oldest (left -> right). Creators are added on the
 * LEFT (newest) and served/removed from the RIGHT (oldest), i.e. FIFO.
 */
export interface WaitingListState {
  /** Fixed cohort capacity, default 10. */
  readonly capacity: number;
  /** Cohorts, newest first. e.g. [8, 10, 10, 6] — the 6 on the right is served next. */
  readonly cohorts: readonly CohortCount[];
}

/** Response shape for the "total waiting" query. */
export interface TotalResponse {
  readonly total: number;
}

/** Error envelope returned on a bad request (e.g. negative N). */
export interface ApiError {
  readonly error: string;
}
