// Pure presentation logic for middle truncation (GH #54): long model names
// used to end-truncate, cutting the suffix — usually the version/variant
// part that distinguishes siblings. splitMiddle cuts a name into a head
// (rendered with CSS ellipsis, shrinks with available width) and a tail
// (fixed, never truncated). The truncation itself is width-driven by CSS;
// this function only decides the split point — no JS character budgeting.
export const MIDDLE_TAIL_KEEP = 12

export interface MiddleSplit {
  head: string
  tail: string
}

// Names of tailKeep + 1 chars or shorter render unsplit: a 1-char head plus
// an ellipsis carries no information, so plain end-truncation is better.
export function splitMiddle(name: string, tailKeep = MIDDLE_TAIL_KEEP): MiddleSplit {
  if (name.length <= tailKeep + 1) return { head: name, tail: '' }
  return { head: name.slice(0, name.length - tailKeep), tail: name.slice(-tailKeep) }
}
