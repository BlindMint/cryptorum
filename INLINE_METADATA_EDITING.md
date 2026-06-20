# Inline Metadata Editing Notes

Inline metadata editing should keep the Book Details page structure visible while turning editable metadata values into controls in place.

Recommended direction:

- Keep the cover, read actions, progress, file path, and tabs visible while editing.
- Replace read-only rows with field-specific editors after the user clicks Edit Metadata.
- Use a single edit state with clear Save and Cancel actions.
- Extract reusable editor pieces before refactoring the layout: author list editor, series editor, metadata suggestion input, and metadata field row.
- Keep less common fields such as ISBN, ASIN, page count, and language in the details grid rather than moving them to a separate form.

Suggested implementation order:

1. Add metadata suggestions and the file path to the current edit view.
2. Extract reusable metadata editor controls from the current edit view.
3. Convert individual detail rows to inline editors.
4. Remove the separate edit-only layout after the inline rows cover the same functionality.

Primary risks:

- The details page can become visually noisy if every field changes shape at once.
- Save and cancel behavior must be obvious, especially when editing fields below the fold.
- Mobile layout needs extra care because the current detail view already has dense cover, actions, and metadata sections.
