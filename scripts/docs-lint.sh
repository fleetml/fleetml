#!/usr/bin/env bash
# Checks the docs for the drift that seven developer reviews kept finding.
# Run it from the repository root (CI's docs-lint job does; it is quick
# enough for a pre-commit hook). It fails, naming file:line, on:
#
#   ports     18080, 19090 or 19100 in a Markdown file other than the ones
#             that list the defaults (docs/ADVANCED.md, docs/CONFIG.md, the
#             CHANGELOG's history and deploy/), except as the saved port,
#             ${FLEETML_REST_PORT:-18080}, ${FLEETML_GRPC_PORT:-19090} or
#             ${FLEETML_S3_PUBLIC_PORT:-19100}, and on a line that sets an app's
#             own gRPC port (serverPort, FLEETML_PORT, FLEETML_SERVER_PORT: the
#             SDKs' default, not the stack's); and in the chart's NOTES.txt
#   secrets   a key on a command line in a fenced block: --enrollment-key,
#             --api-key, -PfleetmlEnrollmentKey=, -fleetmlEnrollmentKey,
#             an enrollment_key value in a sed or echo (a placeholder too: the
#             reader puts the key there), a literal Bearer token
#   versions  a version in the files a release bumps that is not the
#             CHANGELOG's newest
#   changelog a header that is not `## [x.y.z] - YYYY-MM-DD`, a version
#             without its compare link, a list item or paragraph that does
#             not end a sentence, or unbalanced parentheses or backticks
#   gh        a gh command that writes (pr create, release create, ...)
#   golden    a golden block (<!-- golden: id -->) without its bash block or
#             its "You should see" line, or with an id whose text differs
#             from the same id in another file
set -u
perl -e '
use strict; use warnings;
my $fail = 0;
sub bad { my ($where, $what) = @_; print "$where: $what\n"; $fail = 1 }
sub slurp { my $f = shift; open my $h, "<", $f or return undef; local $/; my $s = <$h>; close $h; $s }
my @md = grep { length } split /\0/, `git ls-files -z "*.md" "deploy/helm/fleetml/templates/NOTES.txt"`;

# ports
my %port_ok = map { $_ => 1 } qw(docs/ADVANCED.md docs/CONFIG.md CHANGELOG.md);
for my $f (@md) {
  next if $port_ok{$f} || ($f =~ m{^deploy/} && $f !~ m{NOTES\.txt$});
  my $n = 0;
  for my $line (split /\n/, slurp($f)) {
    $n++;
    (my $l = $line) =~ s/\$\{FLEETML_(?:REST|GRPC|S3_PUBLIC)_PORT:-(?:18080|19090|19100)\}//g;
    next if $l =~ /\bserverPort\b|"FLEETML_PORT"|\bFLEETML_SERVER_PORT\b/;
    bad("$f:$n", "port literal $1 (write \${FLEETML_..._PORT:-$1}, or name the port in words)") if $l =~ /(?<![\w.])(18080|19090|19100)(?!\w)/;
  }
}

# secrets on a command line, in fenced blocks
for my $f (@md) {
  my ($n, $in) = (0, 0);
  for my $line (split /\n/, slurp($f)) {
    $n++;
    if ($line =~ /^\s*```/) { $in = !$in; next }
    next unless $in;
    bad("$f:$n", "key on a command line: $1 (pass it in the environment)")
      if $line =~ /(--enrollment-key\b|--api-key\b|-PfleetmlEnrollmentKey=|-fleetmlEnrollmentKey\b|\b(?:sed|echo)\b.*enrollment_key:|Bearer [A-Za-z0-9_-]{20,})/;
  }
}

# versions
my $cl = slurp("CHANGELOG.md");
my ($release) = $cl =~ /^## \[(\d+\.\d+\.\d+)\] - /m;
my @version_files = (
  ["android/gradle.properties", qr/^sdkVersion=(\S+)$/m],
  ["android/sample-app/build.gradle.kts", qr/versionName = "([^"]+)"/],
  ["dashboard/package.json", qr/^  "version": "([^"]+)"/m],
  ["dashboard/package-lock.json", qr/^(?:  |      )"version": "([^"]+)"/m, 2],
  ["deploy/helm/fleetml/Chart.yaml", qr/^version: (\S+)$/m],
  ["deploy/helm/fleetml/Chart.yaml", qr/^appVersion: "([^"]+)"$/m],
  ["ios/FleetMLSDK/Sources/FleetMLSDK/FleetML.swift", qr/sdkVersion = "([^"]+)"/],
  ["ios/FleetMLSDK/Tests/FleetMLSDKTests/VersionTests.swift", qr/FleetML\.sdkVersion, "([^"]+)"/],
  ["ios/SampleApp/project.yml", qr/MARKETING_VERSION: (\S+)/],
  ["python/src/fleetml/__init__.py", qr/^__version__ = "([^"]+)"/m],
  ["python/tests/test_client.py", qr/__version__ == "([^"]+)"/],
);
# In the docs these are optional: a file need not have them.
for my $f (grep { /\.md$/ && $_ ne "CHANGELOG.md" } @md) { push @version_files, [$f, qr/fleetml-sdk:(\d+\.\d+\.\d+)/], [$f, qr/from: "?(\d+\.\d+\.\d+)"?/], [$f, qr/Up to Next Major Version\*\* from (\d+\.\d+\.\d+)/] }
for my $v (@version_files) {
  my ($f, $re, $count) = @$v;
  my $s = slurp($f); next unless defined $s;
  my @found;
  while ($s =~ /$re/g) { push @found, [$1, substr($s, 0, $-[0]) =~ tr/\n//]; last if $count && @found >= $count }
  bad($f, "no version found ($re)") if !@found && $f !~ /\.md$/;
  for (@found) { bad("$f:" . ($_->[1] + 1), "version $_->[0], the CHANGELOG says $release") if $_->[0] ne $release }
}

# changelog
my $n = 0; my %versions; my $para = ""; my $para_start = 0; my $in = 0;
my $check = sub {
  my ($text, $at) = @_;
  return if $text !~ /\S/ || $text =~ /^\s*(#|\[|```|\|)/;
  (my $t = $text) =~ s/\s+$//;
  bad("CHANGELOG.md:$at", "does not end a sentence: ...\x27" . substr($t, -40) . "\x27") unless $t =~ /[.:!?)`"\x27]$/;
  my $o = () = $t =~ /\(/g; my $c = () = $t =~ /\)/g;
  bad("CHANGELOG.md:$at", "unbalanced parentheses ($o open, $c closed)") if $o != $c;
  my $b = () = $t =~ /`/g;
  bad("CHANGELOG.md:$at", "unbalanced backticks") if $b % 2;
};
for my $line (split /\n/, $cl) {
  $n++;
  if ($line =~ /^```/) { $in = !$in; next }
  next if $in;
  if ($line =~ /^## /) {
    if ($line =~ /^## \[(\d+\.\d+\.\d+)\] - \d{4}-\d{2}-\d{2}$/) { $versions{$1} = $n } elsif ($line ne "## [Unreleased]") { bad("CHANGELOG.md:$n", "header is not ## [x.y.z] - YYYY-MM-DD") }
  }
  if ($line =~ /^\s*$/ || $line =~ /^\s*- / || $line =~ /^#/) { $check->($para, $para_start); $para = ""; $para_start = $n }
  $para .= " " . $line unless $line =~ /^#/;
}
$check->($para, $para_start);
for my $v (keys %versions) { bad("CHANGELOG.md:$versions{$v}", "no compare link [$v]: at the end") unless $cl =~ /^\[\Q$v\E\]: https:/m }

# gh writes
for my $f (@md) {
  my $n = 0;
  for my $line (split /\n/, slurp($f)) {
    $n++;
    bad("$f:$n", "a gh command that writes") if $line =~ /\bgh (?:pr (?:create|merge|comment|edit|close|review)|release (?:create|upload|edit|delete)|workflow run|run rerun|issue (?:create|comment|close|edit)|repo (?:create|edit|delete)|secret set|api (?:-X|--method) (?:POST|PATCH|PUT|DELETE))\b/;
  }
}

# golden blocks
my %golden;
for my $f (@md) {
  my @l = split /\n/, slurp($f);
  for my $i (0 .. $#l) {
    next unless $l[$i] =~ /^(\s*)<!-- golden: (\S+)/;
    my ($ind, $id) = ($1, $2); my $j = $i + 1;
    if ($j > $#l || $l[$j] !~ /^\s*```\w+$/) { bad("$f:" . ($i + 1), "golden $id: no fenced block right after the marker"); next }
    my $k = $j + 1; my @code;
    while ($k <= $#l && $l[$k] !~ /^\s*```$/) { (my $c = $l[$k]) =~ s/^\Q$ind\E//; push @code, $c; $k++ }
    my $m = $k + 1; $m++ while $m <= $#l && $l[$m] !~ /\S/;
    bad("$f:" . ($i + 1), "golden $id: no \"You should see\" line after its block") unless $m <= $#l && $l[$m] =~ /^\s*You should see/;
    my $code = join "\n", @code;
    if (exists $golden{$id} && $golden{$id}[0] ne $code) { bad("$f:" . ($i + 1), "golden $id differs from the one in $golden{$id}[1]") }
    $golden{$id} //= [$code, $f];
  }
}
exit $fail;
'
status=$?
[ "$status" -eq 0 ] && echo "docs-lint: clean"
exit "$status"
