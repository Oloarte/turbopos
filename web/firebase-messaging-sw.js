// Firebase Messaging Service Worker
importScripts('https://www.gstatic.com/firebasejs/10.7.1/firebase-app-compat.js');
importScripts('https://www.gstatic.com/firebasejs/10.7.1/firebase-messaging-compat.js');

firebase.initializeApp({
  apiKey: "AIzaSyD7oTezdatdlev7z--b66UIh48HZWC_5ko",
  authDomain: "turbopos-7f9a2.firebaseapp.com",
  projectId: "turbopos-7f9a2",
  storageBucket: "turbopos-7f9a2.firebasestorage.app",
  messagingSenderId: "778124637274",
  appId: "1:778124637274:web:09ff9eb5ab87ca44e21463"
});

const messaging = firebase.messaging();

messaging.onBackgroundMessage(payload => {
  const {title, body} = payload.notification || {};
  self.registration.showNotification(title || 'TurboPOS', {
    body: body || '',
    icon: '/icon-192.png',
    badge: '/icon-192.png',
    tag: 'turbopos-sale',
    data: payload.data || {}
  });
});
