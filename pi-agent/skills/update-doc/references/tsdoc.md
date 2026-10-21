# TSDoc

- Explain behavior rather than repeating TypeScript types.
- Use `@param` and `@returns` when names and signatures do not communicate the semantics.
- Document constraints, side effects, validation, fallback behavior, and meaningful failure cases.
- Follow project conventions for public API coverage and supported tags.
- Keep comments short; avoid exhaustive parameter lists that add no information.

```typescript
/**
 * Fetches a rating, falling back to the scraper on API failure.
 * Returns null when the request times out or no reviews exist.
 */
async function fetchRating(domain: string): Promise<Rating | null> {}
```

The example illustrates comment syntax, not an implementation. Confirm all stated behavior against the actual code.
