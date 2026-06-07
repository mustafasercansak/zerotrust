/**
 * Helper to generate an inputRef callback for HTML5 standard input validation.
 * Sets custom validation message if the input value is blank.
 */
export function requiredValidator(value: string, message: string) {
  return (el: HTMLInputElement | HTMLTextAreaElement | null) => {
    if (el) {
      el.setCustomValidity(value.trim() ? "" : message);
    }
  };
}
