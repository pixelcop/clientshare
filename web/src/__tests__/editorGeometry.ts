// jsdom has no Range layout APIs; ProseMirror/Floating UI need these even
// when tests drive documented editor commands rather than native typing.
export function editorGeometry() {
  const { scrollBy } = window;
  window.scrollBy = () => {};
  const properties = ['getBoundingClientRect', 'getClientRects'] as const;
  const original = properties.map((key) => Object.getOwnPropertyDescriptor(Range.prototype, key));
  Object.defineProperties(Range.prototype, {
    getBoundingClientRect: { configurable: true, value: () => new DOMRect(0, 0, 10, 20) },
    getClientRects: { configurable: true, value: () => [new DOMRect(0, 0, 10, 20)] },
  });
  return () => {
    window.scrollBy = scrollBy;
    properties.forEach((key, index) => {
      if (original[index]) {
        Object.defineProperty(Range.prototype, key, original[index]);
      } else {
        Reflect.deleteProperty(Range.prototype, key);
      }
    });
  };
}
