// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Run with: node --test examples/hotrod/services/frontend/index.test.js
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { test } = require('node:test');
const { runInNewContext } = require('node:vm');

const html = readFileSync(join(__dirname, 'web_assets/index.html'), 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];
const traceID = '12345678901234567890123456789012';
const traceResponse = `00-${traceID}-1234567890123456-01`;
const ride = { Driver: 'T123456C', ETA: 120000000000 };
const config = { jaeger: 'http://localhost:16686' };

function loadPage(pathname = '/') {
  const requests = [];
  const rows = [];
  const logs = [];
  let click;
  const log = {
    prepend(content) {
      rows.unshift({
        content,
        html(value) { this.content = value; },
        append(value) { this.content += value; },
      });
      return this;
    },
    children() { return rows; },
  };
  function $(selector) {
    if (selector === '.uuid') return { html() {} };
    if (selector === '.hotrod-button') return { click(handler) { click = handler; } };
    if (selector === '#hotrod-log') return log;
    assert.ok(rows.includes(selector), 'unexpected jQuery selector');
    return selector;
  }
  $.ajax = (url, options) => {
    let state = 'pending';
    let data;
    let header;
    const callbacks = [];
    const xhr = {
      done(callback) {
        if (state === 'resolved') callback(data);
        else if (state === 'pending') callbacks.push(callback);
        return this;
      },
      getResponseHeader(name) {
        assert.equal(name, 'traceresponse');
        return header;
      },
    };
    requests.push({
      url,
      options,
      resolve(value, responseHeader = traceResponse) {
        assert.equal(state, 'pending');
        state = 'resolved';
        data = value;
        header = responseHeader;
        if (options.success) options.success(data, 'success', xhr);
        callbacks.forEach(callback => callback(data));
      },
      reject() {
        assert.equal(state, 'pending');
        state = 'rejected';
      },
    });
    return xhr;
  };
  runInNewContext(script, {
    $,
    window: { location: { pathname } },
    console: { log: (...args) => logs.push(args) },
  });
  return {
    requests,
    rows,
    logs,
    dispatch(customer = '123') {
      click({ target: { dataset: { customer } } });
      return requests.at(-1);
    },
  };
}

function assertRide(row, driver = ride.Driver) {
  assert.ok(row.content.includes(`HotROD <b>${driver}</b> arriving in 2min`));
  assert.ok(!row.content.includes('Dispatching a car'));
}

function assertLinks(row, driver = ride.Driver, id = traceID) {
  assert.ok(row.content.includes(`${config.jaeger}/search?limit=20&lookback=1h&service=frontend&tags=%7B%22driver%22%3A%22${driver}%22%7D`));
  assert.ok(row.content.includes(`${config.jaeger}/trace/${id}`));
  assert.equal((row.content.match(/find trace/g) || []).length, 1);
  assert.equal((row.content.match(/open trace/g) || []).length, 1);
}

test('loads config before any click and logs it once', () => {
  const page = loadPage();
  assert.equal(page.requests.length, 1);
  assert.match(page.requests[0].url, /^\/config\?nonse=/);
  assert.equal(page.requests[0].options.method, 'GET');
  page.requests[0].resolve(config);
  assert.deepEqual(page.logs.filter(args => args[0] === 'config'), [['config', config]]);
});

test('renders a ride immediately and appends links when config arrives late', () => {
  const page = loadPage();
  page.dispatch().resolve(ride);
  assertRide(page.rows[0]);
  assert.ok(!page.rows[0].content.includes('<a '));
  page.requests[0].resolve(config);
  assertRide(page.rows[0]);
  assertLinks(page.rows[0]);
});

test('adds links immediately when config arrives before dispatch', () => {
  const page = loadPage();
  page.requests[0].resolve(config);
  page.dispatch().resolve(ride);
  assertRide(page.rows[0]);
  assertLinks(page.rows[0]);
});

test('shares one config request across overlapping and subsequent rides', () => {
  const page = loadPage();
  const firstRequest = page.dispatch('123');
  const firstRow = page.rows[0];
  const secondRequest = page.dispatch('392');
  const secondRow = page.rows[0];
  const secondTraceID = 'abcdefabcdefabcdefabcdefabcdefab';
  secondRequest.resolve({ ...ride, Driver: 'T999999C' }, `00-${secondTraceID}-1234567890123456-01`);
  firstRequest.resolve(ride);
  page.requests[0].resolve(config);
  assertRide(firstRow);
  assertLinks(firstRow);
  assertRide(secondRow, 'T999999C');
  assertLinks(secondRow, 'T999999C', secondTraceID);
  page.dispatch('731').resolve(ride);
  assertLinks(page.rows[0]);
  assert.equal(page.requests.length, 4);
  assert.equal(page.requests.filter(request => request.url.startsWith('/config?')).length, 1);
  assert.equal(page.logs.filter(args => args[0] === 'config').length, 1);
});

for (const configFirst of [false, true]) {
  test(`config failure does not block rides (config first: ${configFirst})`, () => {
    const page = loadPage();
    if (configFirst) page.requests[0].reject();
    page.dispatch().resolve(ride);
    if (!configFirst) page.requests[0].reject();
    page.dispatch().resolve(ride);
    page.rows.forEach(row => {
      assertRide(row);
      assert.ok(!row.content.includes('<a '));
    });
  });
}

for (const value of [null, {}, { jaeger: '' }]) {
  test(`missing Jaeger URL leaves a usable ride result (${JSON.stringify(value)})`, () => {
    const page = loadPage();
    page.dispatch().resolve(ride);
    page.requests[0].resolve(value);
    assertRide(page.rows[0]);
    assert.ok(!page.rows[0].content.includes('<a '));
  });
}

test('missing trace response still provides the trace search link', () => {
  const page = loadPage();
  page.requests[0].resolve(config);
  page.dispatch().resolve(ride, null);
  assertRide(page.rows[0]);
  assert.ok(page.rows[0].content.includes('find trace'));
  assert.ok(!page.rows[0].content.includes('open trace'));
});

test('uses the current base path for config and dispatch', () => {
  const page = loadPage('/hotrod');
  assert.match(page.requests[0].url, /^\/hotrod\/config\?nonse=/);
  const dispatch = page.dispatch('392');
  assert.match(dispatch.url, /^\/hotrod\/dispatch\?customer=392&nonse=/);
  assert.equal(dispatch.options.method, 'GET');
  assert.match(dispatch.options.headers.baggage, /^session=\d+, request=\d+-1$/);
  dispatch.resolve(ride);
  page.requests[0].resolve(config);
  assertLinks(page.rows[0]);
});
