#!/usr/bin/env bash

set -x
set -e

echo "Building docs and checking links with Sphinx"
rm -rf docs/_build
make -C docs html

# Exclude release notes from linkcheck when running in travis.
# Note: we've already rendered these - its safe to remove the unneeded source files.
if [ "$TRAVIS" = true ]; then
  rm -f docs/RELEASE-NOTES.rst
fi
make -C docs linkcheck

echo "Checking grammar and style"
write-good `find docs -name '*.rst'` --passive --so --no-illusion --thereIs --cliches
