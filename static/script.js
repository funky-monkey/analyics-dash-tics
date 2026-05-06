(function () {
  'use strict';

  if (navigator.doNotTrack === '1' || window.doNotTrack === '1') return;

  var script = document.currentScript;
  var siteToken = script && script.getAttribute('data-site');
  if (!siteToken) return;

  var endpoint = (script.getAttribute('data-api') || '/collect');

  function getUTM(param) {
    return new URLSearchParams(window.location.search).get(param) || '';
  }

  function send(type, props) {
    var payload = {
      site: siteToken,
      type: type || 'pageview',
      url: window.location.href,
      referrer: document.referrer,
      width: window.innerWidth,
      language: navigator.language || '',
      utm_source: getUTM('utm_source'),
      utm_medium: getUTM('utm_medium'),
      utm_campaign: getUTM('utm_campaign'),
    };
    if (props) payload.props = props;

    if (navigator.sendBeacon) {
      navigator.sendBeacon(endpoint, JSON.stringify(payload));
    } else {
      var xhr = new XMLHttpRequest();
      xhr.open('POST', endpoint, true);
      xhr.setRequestHeader('Content-Type', 'application/json');
      xhr.send(JSON.stringify(payload));
    }
  }

  send('pageview');

  var origPushState = history.pushState;
  history.pushState = function () {
    origPushState.apply(this, arguments);
    send('pageview');
  };
  window.addEventListener('popstate', function () { send('pageview'); });

  window.analytics = {
    track: function (eventName, props) {
      send(eventName, props || {});
    }
  };
})();
