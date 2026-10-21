# JSDoc

- Use JSDoc to provide JavaScript types and explain behavior.
- Include typed `@param` and `@returns` annotations where the project uses JSDoc typing.
- Describe constraints, optional values, failure cases, and non-obvious return behavior.
- Document public APIs and object shapes; skip trivial private helpers unless project conventions require them.
- Keep descriptions brief and avoid implementation narration.

```javascript
/**
 * Fetches a rating, falling back to the scraper on API failure.
 * @param {string} domain Domain to look up.
 * @returns {Promise<Rating|null>} Null when the domain has no reviews.
 */
async function fetchRating(domain) {}
```

The example illustrates comment syntax, not an implementation. Confirm all stated behavior against the actual code.
