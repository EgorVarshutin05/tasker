import { options as settings, run, summary } from './common.js';
export const options = settings;
export default function () { run(false); }
export function handleSummary(data) { return summary(data); }
