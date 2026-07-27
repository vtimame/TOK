#!/usr/bin/env node
import { readFileSync } from "node:fs";

const [reportPath, baselinePath] = process.argv.slice(2);

if (!reportPath || !baselinePath) {
  console.error("usage: check-jscpd-baseline.mjs <jscpd-report.json> <baseline.json>");
  process.exit(2);
}

const report = JSON.parse(readFileSync(reportPath, "utf8"));
const baseline = JSON.parse(readFileSync(baselinePath, "utf8"));

const failures = [];

function checkMetric(scope, metric, actual, max) {
  if (typeof max !== "number") {
    return;
  }
  if (actual > max) {
    failures.push(`${scope}.${metric}: ${actual} > ${max}`);
  }
}

const total = report.statistics?.total ?? {};
checkMetric("total", "clones", total.clones ?? 0, baseline.maxClones);
checkMetric("total", "duplicatedLines", total.duplicatedLines ?? 0, baseline.maxDuplicatedLines);
checkMetric("total", "duplicatedTokens", total.duplicatedTokens ?? 0, baseline.maxDuplicatedTokens);

for (const [format, formatBaseline] of Object.entries(baseline.formats ?? {})) {
  const stats = report.statistics?.formats?.[format] ?? {};
  checkMetric(format, "clones", stats.clones ?? 0, formatBaseline.maxClones);
  checkMetric(format, "duplicatedLines", stats.duplicatedLines ?? 0, formatBaseline.maxDuplicatedLines);
  checkMetric(format, "duplicatedTokens", stats.duplicatedTokens ?? 0, formatBaseline.maxDuplicatedTokens);
}

if (failures.length > 0) {
  console.error("jscpd baseline regression detected:");
  for (const failure of failures) {
    console.error(` - ${failure}`);
  }
  process.exit(1);
}

console.log(
  `jscpd baseline ok: clones=${total.clones ?? 0}, duplicatedLines=${total.duplicatedLines ?? 0}, duplicatedTokens=${total.duplicatedTokens ?? 0}`,
);
