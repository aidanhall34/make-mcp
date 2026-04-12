module.exports = {
  branches: ['main'],
  tagFormat: 'v${version}',
  plugins: [
    // Determines the next version from commit messages since the last tag.
    // Only the squashed PR commit message (the PR title) is analyzed — individual
    // branch commits are not in the main history and are never seen.
    [
      '@semantic-release/commit-analyzer',
      {
        preset: 'conventionalcommits',
      },
    ],
    // Generates the structured release notes used by both the GitHub release
    // body and the CHANGELOG.md entry below.
    [
      '@semantic-release/release-notes-generator',
      {
        preset: 'conventionalcommits',
      },
    ],
    // Prepends the release notes to CHANGELOG.md, creating the file if it does
    // not exist. Runs before @semantic-release/github so the file is present
    // when the GitHub release is created.
    '@semantic-release/changelog',
    // Creates the GitHub release and attaches any declared assets. Release
    // archives are attached by the separate publish-release-archives job.
    // successComment and failComment are disabled — no issue/PR commenting
    // is needed, which removes the requirement for issues: write and
    // pull-requests: write permissions on the release job.
    [
      '@semantic-release/github',
      {
        assets: [],
        successComment: false,
        failComment: false,
      },
    ],
    // Commits the updated CHANGELOG.md back to main with [skip ci] so the
    // push does not re-trigger the pipeline.
    [
      '@semantic-release/git',
      {
        assets: ['CHANGELOG.md'],
        message: 'chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}',
      },
    ],
  ],
};
