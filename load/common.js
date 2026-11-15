import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const requests = new Counter('requests_total');
const serverErrors = new Counter('http_5xx');
const created = new Counter('created_total');
const warmup = __ENV.WARMUP === '1';
export const options = {
  scenarios: { steady: {
    executor: 'constant-arrival-rate', rate: 50, timeUnit: '1s',
    duration: warmup ? '10s' : '60s',
    preAllocatedVUs: 20, maxVUs: 100, gracefulStop: '2s',
  } },
  thresholds: warmup ? {} : {
    requests_total: ['count>=2940', 'rate>=49'],
    dropped_iterations: ['count==0'],
    http_req_duration: ['p(95)<=200'],
    http_5xx: ['count==0'],
    http_req_failed: ['rate==0'],
    checks: ['rate==1'],
  },
  discardResponseBodies: false,
};
function schema(task) {
  return task !== null && typeof task === 'object' && !Array.isArray(task) &&
    Object.keys(task).sort().join(',') === 'done,id,title' &&
    Number.isInteger(task.id) && task.id >= 1 && task.id <= 2147483647 &&
    typeof task.title === 'string' && typeof task.done === 'boolean';
}
export function run(write) {
  const url = (__ENV.BASE_URL || 'http://127.0.0.1:8080') + '/api/tasks';
  const response = write ? http.post(url, JSON.stringify({title:'Нагрузка'}), {
    headers: {'Content-Type':'application/json'}, redirects: 0, timeout:'5s',
  }) : http.get(url, {redirects:0, timeout:'5s'});
  requests.add(1);
  serverErrors.add(response.status >= 500 && response.status <= 599 ? 1 : 0);
  created.add(response.status === 201 ? 1 : 0);
  let body;
  try { body = response.json(); } catch (_) { body = undefined; }
  check(response, {
    'exact status': r => r.status === (write ? 201 : 200),
    'JSON Content-Type': r => r.headers['Content-Type'] === 'application/json; charset=utf-8',
    'exact JSON contract': () => write ? schema(body) && body.title === 'Нагрузка' && body.done === false : Array.isArray(body) && body.length === 0,
  });
}
export function summary(data) {
  return { [__ENV.SUMMARY_PATH || 'load-summary.json']: JSON.stringify(data, null, 2) };
}
