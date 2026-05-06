/**
 * Tailwind CSS v4 configuration.
 *
 * v4 uses CSS-first configuration — theme tokens and content sources are
 * defined in static/css/input.css via @import "tailwindcss" and @theme.
 *
 * This file is kept for tooling compatibility (e.g. IDE plugins).
 * The standalone CLI reads content globs from the CSS file automatically.
 *
 * @type {import('tailwindcss').Config}
 */
module.exports = {
  content: [
    "./templates/**/*.html",
    "./static/ts/**/*.ts",
  ],
}
