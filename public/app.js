// Frontend for the cohort waiting-list demo.
// Buttons POST/GET the API, then re-render the returned snapshot. Pure DOM, no deps.

const MAX_CELLS = 20; // beyond this, collapse the middle into an ellipsis
const HEAD = 10; // newest cohorts kept on the left
const TAIL = MAX_CELLS - HEAD; // oldest cohorts kept on the right

const viz = document.getElementById("viz");
const summary = document.getElementById("summary");
const errorEl = document.getElementById("error");
const capacityInput = document.getElementById("capacity");
const nInput = document.getElementById("n");

/** Build one cohort cell. `count` of `capacity`; `isServeNext` marks the oldest. */
function cohortCell(count, capacity, isServeNext) {
  const cell = document.createElement("div");
  const partial = count < capacity;
  cell.className =
    "cohort" + (partial ? " cohort--partial" : "") + (isServeNext ? " cohort--serve" : "");
  // Height scales with fill: 24px floor + up to ~60px by ratio.
  const ratio = capacity > 0 ? count / capacity : 1;
  cell.style.height = `${24 + Math.round(ratio * 60)}px`;
  cell.textContent = String(count);
  cell.title = `${count} / ${capacity}${isServeNext ? " — served next" : ""}`;
  return cell;
}

function ellipsisCell(hiddenCount) {
  const el = document.createElement("span");
  el.className = "ellipsis";
  el.textContent = "…";
  el.title = `${hiddenCount} more cohort(s) hidden`;
  return el;
}

/** Render a WaitingListState: array of numbers, middle-collapsed past 20 cells. */
function render(state) {
  const { cohorts, capacity } = state;
  viz.replaceChildren();

  if (cohorts.length === 0) {
    const note = document.createElement("span");
    note.className = "empty-note";
    note.textContent = "[] — empty waiting list";
    viz.append(note);
    return;
  }

  const lastIndex = cohorts.length - 1; // oldest = served next
  const overflowing = cohorts.length > MAX_CELLS;
  const head = overflowing ? cohorts.slice(0, HEAD) : cohorts;
  const tail = overflowing ? cohorts.slice(cohorts.length - TAIL) : [];

  head.forEach((count, i) => {
    viz.append(cohortCell(count, capacity, !overflowing && i === lastIndex));
  });

  if (overflowing) {
    const hidden = cohorts.length - HEAD - TAIL;
    viz.append(ellipsisCell(hidden));
    const tailStart = cohorts.length - TAIL;
    tail.forEach((count, i) => {
      viz.append(cohortCell(count, capacity, tailStart + i === lastIndex));
    });
  }
}

function renderSummary(state) {
  const total = state.total ?? state.cohorts.reduce((sum, c) => sum + c, 0);
  const arr = "[" + state.cohorts.join(", ") + "]";
  let html = `${arr} · cohorts: <strong>${state.cohorts.length}</strong> · total waiting: <strong>${total}</strong> · capacity: ${state.capacity}`;
  // Real-backend signals (absent in the old static demo).
  if (typeof state.inFlight === "number") {
    html += ` · in-flight: <strong>${state.inFlight}</strong>`;
  }
  if (typeof state.totalAdded === "number") {
    html += ` · lifetime added: <strong>${state.totalAdded}</strong>`;
  }
  summary.innerHTML = html;
}

async function call(op) {
  // "Total" is a live refresh: re-read the full state (viz + summary), since the
  // k8s workers drain the queue asynchronously.
  if (op === "total") {
    return loadState();
  }

  errorEl.textContent = "";
  try {
    let res;
    if (op === "create") {
      res = await fetch("/api/create", postJson({ capacity: numOrUndef(capacityInput.value) }));
    } else {
      res = await fetch(`/api/${op}`, postJson({ n: numOrUndef(nInput.value) }));
    }

    const data = await res.json();
    if (!res.ok) {
      errorEl.textContent = data.error || `Request failed (${res.status}).`;
      return;
    }
    render(data);
    renderSummary(data);
  } catch (err) {
    errorEl.textContent = "Network error — is the backend running?";
    console.error(err);
  }
}

function postJson(body) {
  return {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  };
}

function numOrUndef(value) {
  const n = Number(value);
  return value === "" || Number.isNaN(n) ? undefined : n;
}

/** Non-destructive load of the live queue (used on page load / refresh). */
async function loadState() {
  errorEl.textContent = "";
  try {
    const res = await fetch("/api/state");
    const data = await res.json();
    if (!res.ok) {
      errorEl.textContent = data.error || `Request failed (${res.status}).`;
      return;
    }
    render(data);
    renderSummary(data);
  } catch (err) {
    errorEl.textContent = "Network error — is the backend running?";
    console.error(err);
  }
}

document.querySelectorAll("button[data-op]").forEach((btn) => {
  btn.addEventListener("click", () => call(btn.dataset.op));
});

// Initial paint: read the live queue WITHOUT mutating it (no create-on-load).
loadState();
