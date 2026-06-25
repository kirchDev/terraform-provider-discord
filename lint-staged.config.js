export default {
  '*.md': (filenames) => {
    // Skip files oxfmt ignores via .prettierignore: generated docs/ (tfplugindocs)
    // and CHANGELOG.md (release-please). oxfmt errors when handed only ignored
    // files, so a docs-only commit would otherwise fail the hook. README.md keeps
    // its hand-authored house-style formatting.
    const files = filenames.filter(
      (f) =>
        !f.endsWith('README.md') &&
        !/(^|\/)docs\//.test(f) &&
        !f.endsWith('CHANGELOG.md')
    );
    return files.length > 0 ? `pnpm exec oxfmt ${files.join(' ')}` : [];
  },
  '*.{json,jsonc,yml,yaml}': (filenames) => {
    const files = filenames.filter((f) => !f.includes('pnpm-lock.yaml'));
    return files.length > 0 ? `pnpm exec oxfmt ${files.join(' ')}` : [];
  },
  '*.{js,ts,mjs,cjs}': (filenames) => [
    `pnpm exec oxlint --fix --deny-warnings ${filenames.join(' ')}`,
    `pnpm exec oxfmt ${filenames.join(' ')}`
  ]
};
