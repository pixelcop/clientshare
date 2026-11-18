// Keep the matching tiers in sync with internal/services/people_search.go.
export function normalizePeopleSearch(value: string) {
  return value
    .trim()
    .normalize('NFD')
    .replace(/\p{Mn}/gu, '')
    .toLowerCase();
}

function oneEdit(query: string[], word: string[]) {
  if (query.length < 3 || Math.abs(query.length - word.length) > 1) {
    return false;
  }
  const [a, b] = query.length > word.length ? [word, query] : [query, word];
  let i = 0;
  while (i < a.length && a[i] === b[i]) {
    i += 1;
  }
  const tailMatches = (start: number, offset: number) =>
    a.slice(start).every((char, index) => char === b[start + index + offset]);
  if (a.length < b.length) {
    return tailMatches(i, 1);
  }
  return (
    i < a.length &&
    (tailMatches(i + 1, 0) || (a[i] === b[i + 1] && a[i + 1] === b[i] && tailMatches(i + 2, 0)))
  );
}

export const noPeopleMatch = 4;

function matchField(query: string, field: string) {
  const value = normalizePeopleSearch(field);
  if (value === query) {
    return 0;
  }
  const words = value.split(/[^\p{L}\p{N}]+/u);
  if (value.startsWith(query) || words.some((word) => word.startsWith(query))) {
    return 1;
  }
  if (value.includes(query)) {
    return 2;
  }
  const chars = Array.from(query);
  return words.some((word) => oneEdit(chars, Array.from(word))) ? 3 : noPeopleMatch;
}

export function peopleMatch(query: string, ...fields: string[]) {
  return query ? Math.min(...fields.map((field) => matchField(query, field))) : 0;
}
