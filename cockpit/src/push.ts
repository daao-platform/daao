/**
 * Push Notifications for DAAO
 * 
 * Implements Web Push API integration for DMS triggers,
 * INPUT_REQUIRED state, and session events. Uses VAPID
 * for subscription management.
 * 
 * Supports mobile browsers (iOS Safari 16.4+, Chrome Android)
 */

export interface PushNotificationConfig {
  vapidPublicKey: string;
  subscriptionEndpoint?: string;
  onDmsTriggered?: (sessionId: string) => void;
  onInputRequired?: (sessionId: string, message: string) => void;
  onSessionTerminated?: (sessionId: string, reason: string) => void;
  onSubscriptionChange?: (subscription: PushSubscription | null) => void;
}

export interface PushSubscriptionData {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
  expirationTime: number | null;
}

export interface NotificationPayload {
  title: string;
  body: string;
  icon?: string;
  badge?: string;
  tag?: string;
  data?: any;
  requireInteraction?: boolean;
  vibrate?: number[];
}

/**
 * Session event types for push notifications
 */
export enum SessionEventType {
  RUNNING = 'RUNNING',
  SUSPENDED = 'SUSPENDED',
  TERMINATED = 'TERMINATED',
  INPUT_REQUIRED = 'INPUT_REQUIRED',
  DMS_TRIGGERED = 'DMS_TRIGGERED',
}

/**
 * Push notification event types
 */
export enum PushEventType {
  DMS_SUSPENSION = 'DMS_SUSPENSION',
  INPUT_REQUIRED = 'INPUT_REQUIRED',
  SESSION_TERMINATED = 'SESSION_TERMINATED',
  SESSION_RESUMED = 'SESSION_RESUMED',
}

/**
 * PushNotificationManager handles Web Push API integration
 * with VAPID subscription management
 */
export class PushNotificationManager {
  private config: PushNotificationConfig;
  private subscription: PushSubscription | null = null;
  private registration: ServiceWorkerRegistration | null = null;

  constructor(config: PushNotificationConfig) {
    this.config = config;
  }

  /**
   * Check if push notifications are supported
   */
  static isSupported(): boolean {
    if (typeof window === 'undefined') {
      return false;
    }
    return 'serviceWorker' in navigator && 'PushManager' in window;
  }

  /**
   * Check if notification permission is granted
   */
  static isPermissionGranted(): boolean {
    if (typeof window === 'undefined') {
      return false;
    }
    return Notification.permission === 'granted';
  }

  /**
   * Request notification permission from user
   */
  static async requestPermission(): Promise<NotificationPermission> {
    if (typeof window === 'undefined') {
      return 'denied';
    }

    if (!('Notification' in window)) {
      return 'denied';
    }

    const currentPermission = Notification.permission;
    if (currentPermission === 'granted') {
      return 'granted';
    }

    if (currentPermission !== 'denied') {
      const permission = await Notification.requestPermission();
      return permission;
    }

    return 'denied';
  }

  /**
   * Initialize push notifications and register service worker
   */
  async initialize(): Promise<boolean> {
    if (!PushNotificationManager.isSupported()) {
      console.warn('Push notifications not supported in this browser');
      return false;
    }

    try {
      // Register service worker
      this.registration = await navigator.serviceWorker.register('/sw.js');

      // Check existing subscription
      const existingSubscription = await this.registration.pushManager.getSubscription();
      if (existingSubscription) {
        this.subscription = existingSubscription;
        this.config.onSubscriptionChange?.(existingSubscription);
      }

      return true;
    } catch (error) {
      console.error('Failed to initialize push notifications:', error);
      return false;
    }
  }

  /**
   * Subscribe to push notifications using VAPID key
   */
  async subscribe(): Promise<PushSubscription | null> {
    if (!this.registration) {
      await this.initialize();
    }

    if (!this.registration) {
      console.error('Service worker not registered');
      return null;
    }

    try {
      // Convert VAPID public key from base64 to Uint8Array
      const applicationServerKey = this.urlBase64ToUint8Array(this.config.vapidPublicKey);

      // Subscribe with VAPID key
      const subscription = await this.registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: applicationServerKey as BufferSource,
      });

      this.subscription = subscription;
      this.config.onSubscriptionChange?.(subscription);

      // Send subscription to server for storage
      await this.sendSubscriptionToServer(subscription);

      return subscription;
    } catch (error) {
      console.error('Failed to subscribe to push notifications:', error);
      return null;
    }
  }

  /**
   * Unsubscribe from push notifications
   */
  async unsubscribe(): Promise<boolean> {
    if (!this.subscription) {
      return true;
    }

    try {
      const success = await this.subscription.unsubscribe();
      if (success) {
        this.subscription = null;
        this.config.onSubscriptionChange?.(null);
      }
      return success;
    } catch (error) {
      console.error('Failed to unsubscribe:', error);
      return false;
    }
  }

  /**
   * Get current subscription
   */
  getSubscription(): PushSubscription | null {
    return this.subscription;
  }

  /**
   * Send subscription data to the server
   * Sends the subscription along with VAPID key to the backend
   */
  private async sendSubscriptionToServer(subscription: PushSubscription): Promise<void> {
    const subscriptionData: PushSubscriptionData = {
      endpoint: subscription.endpoint,
      keys: {
        p256dh: subscription.toJSON().keys?.p256dh || '',
        auth: subscription.toJSON().keys?.auth || '',
      },
      expirationTime: subscription.expirationTime,
    };

    // Send subscription to server with VAPID key for storage
    // The API client will include the VAPID public key in the request
    try {
      const { subscribeToPushNotifications } = await import('./api/client');
      await subscribeToPushNotifications(subscriptionData, this.config.vapidPublicKey);
      console.log('Push subscription saved to server');
    } catch (error) {
      console.error('Failed to send subscription to server:', error);
      // Fallback to local storage for offline support
      localStorage.setItem('push_subscription', JSON.stringify(subscriptionData));
    }
  }

  /**
   * Handle DMS (Dead Man's Switch) suspension notification
   * Called when a session is about to be suspended due to inactivity
   */
  async handleDmsTriggered(sessionId: string): Promise<void> {
    // Trigger callback
    this.config.onDmsTriggered?.(sessionId);

    // Show local notification for DMS suspension
    const payload: NotificationPayload = {
      title: 'Session Suspending',
      body: `Session ${sessionId} is about to suspend due to inactivity.`,
      icon: '/icons/warning.png',
      tag: `dms-${sessionId}`,
      data: {
        type: PushEventType.DMS_SUSPENSION,
        sessionId,
      },
      requireInteraction: true,
    };

    await this.showNotification(payload);
  }

  /**
   * Handle INPUT_REQUIRED notification
   * Called when the agent needs user input to continue
   */
  async handleInputRequired(sessionId: string, message: string): Promise<void> {
    // Trigger callback
    this.config.onInputRequired?.(sessionId, message);

    // Show local notification for input required
    const payload: NotificationPayload = {
      title: 'Input Required',
      body: message || `Session ${sessionId} needs your input`,
      icon: '/icons/input.png',
      tag: `input-${sessionId}`,
      data: {
        type: PushEventType.INPUT_REQUIRED,
        sessionId,
        message,
      },
      requireInteraction: true,
      vibrate: [200, 100, 200],
    };

    await this.showNotification(payload);
  }

  /**
   * Handle session terminated notification
   * Called when a session has been terminated
   */
  async handleSessionTerminated(sessionId: string, reason: string): Promise<void> {
    // Trigger callback
    this.config.onSessionTerminated?.(sessionId, reason);

    // Show local notification for session terminated
    const payload: NotificationPayload = {
      title: 'Session Terminated',
      body: reason || `Session ${sessionId} has ended`,
      icon: '/icons/terminated.png',
      tag: `terminated-${sessionId}`,
      data: {
        type: PushEventType.SESSION_TERMINATED,
        sessionId,
        reason,
      },
    };

    await this.showNotification(payload);
  }

  /**
   * Handle incoming push message from service worker
   */
  async handlePushMessage(event: { data: { json(): any } | null }): Promise<void> {
    if (!event.data) {
      return;
    }

    try {
      const data = event.data.json();

      switch (data.type) {
        case PushEventType.DMS_SUSPENSION:
          await this.handleDmsTriggered(data.sessionId);
          break;
        case PushEventType.INPUT_REQUIRED:
          await this.handleInputRequired(data.sessionId, data.message);
          break;
        case PushEventType.SESSION_TERMINATED:
          await this.handleSessionTerminated(data.sessionId, data.reason);
          break;
        default:
          console.warn('Unknown push event type:', data.type);
      }
    } catch (error) {
      console.error('Failed to handle push message:', error);
    }
  }

  /**
   * Show a local notification (when app is in foreground)
   */
  async showNotification(payload: NotificationPayload): Promise<Notification | null> {
    if (!('Notification' in window) || Notification.permission !== 'granted') {
      return null;
    }

    try {
      const notification = new Notification(payload.title, {
        body: payload.body,
        icon: payload.icon,
        badge: payload.badge,
        tag: payload.tag,
        data: payload.data,
        requireInteraction: payload.requireInteraction,
      } as NotificationOptions);

      notification.onclick = () => {
        window.focus();
        notification.close();
      };

      // Auto-close after 5 seconds unless requireInteraction is true
      if (!payload.requireInteraction) {
        setTimeout(() => notification.close(), 5000);
      }

      return notification;
    } catch (error) {
      console.error('Failed to show notification:', error);
      return null;
    }
  }

  /**
   * Convert VAPID base64 key to Uint8Array
   */
  private urlBase64ToUint8Array(base64String: string): Uint8Array {
    const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
    const base64 = (base64String + padding)
      .replace(/-/g, '+')
      .replace(/_/g, '/');

    const rawData = window.atob(base64);
    const outputArray = new Uint8Array(rawData.length);

    for (let i = 0; i < rawData.length; ++i) {
      outputArray[i] = rawData.charCodeAt(i);
    }

    return outputArray;
  }
}

/**
 * Create a new PushNotificationManager instance
 */
export function createPushNotificationManager(
  config: PushNotificationConfig
): PushNotificationManager {
  return new PushNotificationManager(config);
}

// Default export
export default PushNotificationManager;
