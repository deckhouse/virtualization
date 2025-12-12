//go:build cgo

package usbredir

/*
// Initially copied from https://gitlab.freedesktop.org/spice/usbredir/-/blob/usbredir-0.15.0/tools/usbredirect.c?ref_type=tags

#cgo pkg-config: libusbredirhost libusbredirparser-0.5 libusb-1.0 glib-2.0 gio-2.0

#define G_LOG_DOMAIN "usbredirect"
#define G_LOG_USE_STRUCTURED

#include <usbredirhost.h>
#include <usbredirparser.h>
#include <usbredirproto.h>
#include <glib.h>
#include <gio/gio.h>
#include <libusb.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>

#ifdef G_OS_UNIX
#include <glib-unix.h>
#endif


typedef struct device {
	int vendor;
	int product;
	int bus;
	int device_number;
} device;

typedef struct net_settings {
	char *addr;
	int port;
	bool keepalive;
} net_settings;

typedef struct redirect {
   	device device;
	net_settings net_settings;

	bool by_bus;
	bool is_client;
	bool watch_inout;
	int verbosity;

	struct usbredirhost *usbredirhost;
		GSocketConnection *connection;
		GThread *event_thread;
		int event_thread_run;
		int watch_server_id;
		GIOChannel *io_channel;

		GMainLoop *main_loop;
} redirect;

static void create_watch(redirect *self);

typedef struct {
   	device device;
	net_settings net_settings;
    int verbosity;
} usbredir_config;


static bool
by_bus(device *device) {
	return device != NULL && device->bus > 0 && device->device_number > 0;
}

static bool
validate_device_from_config(usbredir_config *config)
{
    bool by_bus_flag = config->device.bus > 0 && config->device.device_number > 0;
    if (by_bus_flag) {
        return config->device.bus > 0 && config->device.device_number > 0;
    }

    if (config->device.vendor <= 0 || config->device.vendor > 0xffff ||
        config->device.product < 0 || config->device.product > 0xffff) {
        return false;
    }

    return true;
}

static bool
is_valid_config(usbredir_config *config) {
    if (!config || !config->net_settings.addr) {
        return false;
    }

    if (!validate_device_from_config(config)) {
        return false;
    }

    return true;
}

static redirect *
new_redirect(usbredir_config *config)
{
	redirect *self = NULL;
	if (!is_valid_config(config)) {
		return self;
	}

	self = g_new0(redirect, 1);
	self->watch_inout = true;
	self->device = config->device;
	self->by_bus = by_bus(&self->device);
	self->is_client = true;
	self->net_settings = config->net_settings;

	return self;
}

static gpointer
thread_handle_libusb_events(gpointer user_data)
{
    redirect *self = (redirect *) user_data;

    int res = 0;
    const char *desc = "";
    while (g_atomic_int_get(&self->event_thread_run)) {
        res = libusb_handle_events(NULL);
        if (res && res != LIBUSB_ERROR_INTERRUPTED) {
            desc = libusb_strerror(res);
            g_warning("Error handling USB events: %s [%i]", desc, res);
            break;
        }
    }
    if (self->event_thread_run) {
        g_debug("%s: the thread aborted, %s(%d)", __FUNCTION__, desc, res);
    }
    return NULL;
}

#if LIBUSBX_API_VERSION >= 0x01000107
static void LIBUSB_CALL
debug_libusb_cb(libusb_context *ctx, enum libusb_log_level level, const char *msg)
{
    GLogLevelFlags glog_level;

    switch(level) {
    case LIBUSB_LOG_LEVEL_ERROR:
        glog_level = G_LOG_LEVEL_ERROR;
        break;
    case LIBUSB_LOG_LEVEL_WARNING:
        glog_level = G_LOG_LEVEL_WARNING;
        break;
    case LIBUSB_LOG_LEVEL_INFO:
        glog_level = G_LOG_LEVEL_INFO;
        break;
    case LIBUSB_LOG_LEVEL_DEBUG:
        glog_level = G_LOG_LEVEL_DEBUG;
        break;
    default:
        g_warn_if_reached();
        return;
    }

    // Do not print the '\n' line feed
    size_t len = strlen(msg);
    len = (msg[len - 1] == '\n') ? len - 1 : len;
    g_log_structured(G_LOG_DOMAIN, glog_level, "MESSAGE", "%.*s", len - 1, msg);
}
#endif

static void
usbredir_log_cb(void *priv, int level, const char *msg)
{
    GLogLevelFlags glog_level;

    switch(level) {
    case usbredirparser_error:
        glog_level = G_LOG_LEVEL_ERROR;
        break;
    case usbredirparser_warning:
        glog_level = G_LOG_LEVEL_WARNING;
        break;
    case usbredirparser_info:
        glog_level = G_LOG_LEVEL_INFO;
        break;
    case usbredirparser_debug:
    case usbredirparser_debug_data:
        glog_level = G_LOG_LEVEL_DEBUG;
        break;
    default:
        g_warn_if_reached();
        return;
    }
    g_log_structured(G_LOG_DOMAIN, glog_level, "MESSAGE", msg);
}

static void
update_watch(redirect *self)
{
    const bool watch_inout = usbredirhost_has_data_to_write(self->usbredirhost) != 0;
    if (watch_inout == self->watch_inout) {
        return;
    }
    g_clear_pointer(&self->io_channel, g_io_channel_unref);
    g_source_remove(self->watch_server_id);
    self->watch_server_id = 0;
    self->watch_inout = watch_inout;

    create_watch(self);
}


static int
usbredir_read_cb(void *priv, uint8_t *data, int count)
{
    redirect *self = (redirect *) priv;
    GIOStream *iostream = G_IO_STREAM(self->connection);
    GError *err = NULL;

    GPollableInputStream *instream = G_POLLABLE_INPUT_STREAM(g_io_stream_get_input_stream(iostream));
    gssize nbytes = g_pollable_input_stream_read_nonblocking(instream,
            data,
            count,
            NULL,
            &err);
    if (nbytes <= 0) {
        if (g_error_matches(err, G_IO_ERROR, G_IO_ERROR_WOULD_BLOCK)) {
            // Try again later
            nbytes = 0;
        } else {
            if (err != NULL) {
                g_warning("Failure at %s: %s", __func__, err->message);
            }
            g_main_loop_quit(self->main_loop);
        }
        g_clear_error(&err);
    }
    return nbytes;
}

static int
usbredir_write_cb(void *priv, uint8_t *data, int count)
{
    redirect *self = (redirect *) priv;
    GIOStream *iostream = G_IO_STREAM(self->connection);
    GError *err = NULL;

    GPollableOutputStream *outstream = G_POLLABLE_OUTPUT_STREAM(g_io_stream_get_output_stream(iostream));
    gssize nbytes = g_pollable_output_stream_write_nonblocking(outstream,
            data,
            count,
            NULL,
            &err);
    if (nbytes <= 0) {
        if (g_error_matches(err, G_IO_ERROR, G_IO_ERROR_WOULD_BLOCK)) {
            // Try again later
            nbytes = 0;
            update_watch(self);
        } else {
            if (err != NULL) {
                g_warning("Failure at %s: %s", __func__, err->message);
            }
            g_main_loop_quit(self->main_loop);
        }
        g_clear_error(&err);
    }
    return nbytes;
}

static void
usbredir_write_flush_cb(void *user_data)
{
    redirect *self = (redirect *) user_data;
    if (!self || !self->usbredirhost) {
        return;
    }

    int ret = usbredirhost_write_guest_data(self->usbredirhost);
    if (ret < 0) {
        g_critical("%s: Failed to write to guest", __func__);
        g_main_loop_quit(self->main_loop);
    }
}

static void
*usbredir_alloc_lock(void)
{
    GMutex *mutex;

    mutex = g_new0(GMutex, 1);
    g_mutex_init(mutex);

    return mutex;
}

static void
usbredir_free_lock(void *user_data)
{
    GMutex *mutex = user_data;

    g_mutex_clear(mutex);
    g_free(mutex);
}

static void
usbredir_lock_lock(void *user_data)
{
    GMutex *mutex = user_data;

    g_mutex_lock(mutex);
}

static void
usbredir_unlock_lock(void *user_data)
{
    GMutex *mutex = user_data;

    g_mutex_unlock(mutex);
}

static gboolean
connection_handle_io_cb(GIOChannel *source, GIOCondition condition, gpointer user_data)
{
    redirect *self = (redirect *) user_data;

    if (condition & G_IO_ERR || condition & G_IO_HUP) {
        g_warning("Connection: err=%d, hup=%d - exiting", (condition & G_IO_ERR), (condition & G_IO_HUP));
        goto end;
    }

    if (condition & G_IO_IN) {
        int ret = usbredirhost_read_guest_data(self->usbredirhost);
        if (ret < 0) {
            g_critical("%s: Failed to read guest", __func__);
            goto end;
        }
    }
    // try to write data in any case, to avoid having another iteration and
    // creation of another watch if there is space in output buffer
    if (usbredirhost_has_data_to_write(self->usbredirhost) != 0) {
        int ret = usbredirhost_write_guest_data(self->usbredirhost);
        if (ret < 0) {
            g_critical("%s: Failed to write to guest", __func__);
            goto end;
        }
    }

    // update the watch if needed
    update_watch(self);
    return G_SOURCE_CONTINUE;

end:
    g_main_loop_quit(self->main_loop);
    return G_SOURCE_REMOVE;
}

static void
create_watch(redirect *self)
{
    GSocket *socket = g_socket_connection_get_socket(self->connection);
    int socket_fd = g_socket_get_fd(socket);

    g_assert_null(self->io_channel);
    self->io_channel =
#ifdef G_OS_UNIX
        g_io_channel_unix_new(socket_fd);
#else
        g_io_channel_win32_new_socket(socket_fd);
#endif

    g_assert_cmpint(self->watch_server_id, ==, 0);
    self->watch_server_id = g_io_add_watch(self->io_channel,
            G_IO_IN | G_IO_HUP | G_IO_ERR | (self->watch_inout ? G_IO_OUT : 0),
            connection_handle_io_cb,
            self);
}

static bool
can_claim_usb_device(libusb_device *dev, libusb_device_handle **handle)
{
    int ret = libusb_open(dev, handle);
    if (ret != 0) {
        g_debug("Failed to open device");
        return false;
    }

    // Opening is not enough. We need to check if device can be claimed
    // for I/O operations
    struct libusb_config_descriptor *config = NULL;
    ret = libusb_get_active_config_descriptor(dev, &config);
    if (ret != 0 || config == NULL) {
        g_debug("Failed to get active descriptor");
        goto fail;
    }

#if LIBUSBX_API_VERSION >= 0x01000102
    libusb_set_auto_detach_kernel_driver(*handle, 1);
#endif

    int i;
    for (i = 0; i < config->bNumInterfaces; i++) {
        int interface_num = config->interface[i].altsetting[0].bInterfaceNumber;
#if LIBUSBX_API_VERSION < 0x01000102
        ret = libusb_detach_kernel_driver(*handle, interface_num);
        if (ret != 0 && ret != LIBUSB_ERROR_NOT_FOUND
            && ret != LIBUSB_ERROR_NOT_SUPPORTED) {
            g_error("failed to detach driver from interface %d: %s",
                    interface_num, libusb_error_name(ret));
            goto fail;
        }
#endif
        ret = libusb_claim_interface(*handle, interface_num);
        if (ret != 0) {
            g_debug("Could not claim interface");
            goto fail;
        }
        ret = libusb_release_interface(*handle, interface_num);
        if (ret != 0) {
            g_debug("Could not release interface");
            goto fail;
        }
    }

    libusb_free_config_descriptor(config);
    return true;

fail:
    libusb_free_config_descriptor(config);
    libusb_close(*handle);
    *handle = NULL;
    return false;
}

static libusb_device_handle *
open_usb_device(redirect *self)
{
    struct libusb_device **devs;
    struct libusb_device_handle *dev_handle = NULL;
    size_t i, ndevices;

    ndevices = libusb_get_device_list(NULL, &devs);
    for (i = 0; i < ndevices; i++) {
        struct libusb_device_descriptor desc;
        if (libusb_get_device_descriptor(devs[i], &desc) != 0) {
            g_warning("Failed to get descriptor");
            continue;
        }

        if (self->by_bus &&
            (self->device.bus != libusb_get_bus_number(devs[i]) ||
             self->device.device_number != libusb_get_device_address(devs[i]))) {
             continue;
        }

        if (!self->by_bus &&
            (self->device.vendor != desc.idVendor ||
             self->device.product != desc.idProduct)) {
             continue;
        }

        if (can_claim_usb_device(devs[i], &dev_handle)) {
            break;
        }
    }

    libusb_free_device_list(devs, 1);
    return dev_handle;
}

static gboolean
connection_incoming_cb(GSocketService    *service,
                       GSocketConnection *client_connection,
                       GObject           *source_object,
                       gpointer           user_data)
{
    redirect *self = (redirect *) user_data;

    // Check if there is already an active connection
    if (self->connection != NULL) {
        g_warning("Rejecting new connection: already connected to a client");
        return G_SOURCE_REMOVE;
    }

    self->connection = g_object_ref(client_connection);

    // Add a GSource watch to handle polling for us and handle IO in the callback
    GSocket *connection_socket = g_socket_connection_get_socket(self->connection);
    g_socket_set_keepalive(connection_socket, self->net_settings.keepalive);
    create_watch(self);
    return G_SOURCE_REMOVE;
}

int usbredir_run(usbredir_config *config)
{
    GError *err = NULL;

    if (libusb_init(NULL)) {
        g_warning("Could not init libusb\n");
        goto err_init;
    }

	redirect *self = new_redirect(config);
	if (!self) {
		goto err_init;
	}

#if LIBUSBX_API_VERSION >= 0x01000107
 	// This was introduced in 1.0.23
    libusb_set_log_cb(NULL, debug_libusb_cb, LIBUSB_LOG_CB_GLOBAL);
#endif

#ifdef G_OS_WIN32
    // WinUSB is the default by backwards compatibility so this is needed to
	// switch to USBDk backend.
#   if LIBUSBX_API_VERSION >= 0x01000106
	libusb_set_option(NULL, LIBUSB_OPTION_USE_USBDK);
#   endif
#endif

    // TODO: setup handle signals


    libusb_device_handle *device_handle = open_usb_device(self);
    if (!device_handle) {
        g_printerr("Failed to open device!\n");
        goto err_init;
    }


	// As per doc below, we are not using hotplug so we must first call
	// libusb_open() and then we can start the event thread.
	//
	//      http://libusb.sourceforge.net/api-1.0/group__libusb__asyncio.html#eventthread
	//
	// The event thread is a must for Windows while on Unix we would ge okay
	// getting the fds and polling oursevelves.
    g_atomic_int_set(&self->event_thread_run, TRUE);
    self->event_thread = g_thread_try_new("usbredirect-libusb-event-thread",
            thread_handle_libusb_events,
            self,
            &err);
    if (!self->event_thread) {
        g_warning("Error starting event thread: %s", err->message);
        libusb_close(device_handle);
        goto err_init;
    }

    self->usbredirhost = usbredirhost_open_full(NULL,
            device_handle,
            usbredir_log_cb,
            usbredir_read_cb,
            usbredir_write_cb,
            usbredir_write_flush_cb,
            usbredir_alloc_lock,
            usbredir_lock_lock,
            usbredir_unlock_lock,
            usbredir_free_lock,
            self,
            "usbredir-go/1.0",
            self->verbosity,
            0);
    if (!self->usbredirhost) {
        g_warning("Error starting usbredirhost");
        goto err_init;
    }


    // Only allow libusb logging if log verbosity is uredirparser_debug_data
    // (or higher), otherwise we disable it here while keeping usbredir's logs enable.
    if (config->verbosity < usbredirparser_debug_data) {
#if LIBUSBX_API_VERSION >= 0x01000106
        int ret = libusb_set_option(NULL, LIBUSB_OPTION_LOG_LEVEL, LIBUSB_LOG_LEVEL_NONE);
        if (ret != LIBUSB_SUCCESS) {
            g_warning("error disabling libusb log level: %s", libusb_error_name(ret));
            goto end;
        }
#else
        libusb_set_debug(NULL, LIBUSB_LOG_LEVEL_NONE);
#endif
    }

    if (self->is_client) {
		// Connect to a remote sever using usbredir to redirect the usb device
        GSocketClient *client = g_socket_client_new();
        self->connection = g_socket_client_connect_to_host(client,
                self->net_settings.addr,
                self->net_settings.port,
                NULL,
                &err);
        g_object_unref(client);
        if (err != NULL) {
            g_warning("Failed to connect to the server: %s", err->message);
            goto end;
        }

        GSocket *connection_socket = g_socket_connection_get_socket(self->connection);
        g_socket_set_keepalive(connection_socket, self->net_settings.keepalive);
        create_watch(self);
    } else {
        GSocketService *socket_service;

        socket_service = g_socket_service_new();
        GInetAddress *iaddr = g_inet_address_new_from_string(self->net_settings.addr);
        if (iaddr == NULL) {
            g_warning("Failed to parse IP: %s", self->net_settings.addr);
            goto end;
        }

        GSocketAddress *saddr = g_inet_socket_address_new(iaddr, self->net_settings.port);
        g_object_unref(iaddr);

        g_socket_listener_add_address(G_SOCKET_LISTENER(socket_service),
                saddr,
                G_SOCKET_TYPE_STREAM,
                G_SOCKET_PROTOCOL_TCP,
                NULL,
                NULL,
                &err);
        if (err != NULL) {
            g_warning("Failed to run as TCP server: %s", err->message);
            goto end;
        }

        g_signal_connect(socket_service,
                "incoming", G_CALLBACK(connection_incoming_cb),
                self);
    }

	self->main_loop = g_main_loop_new(NULL, FALSE);
	g_main_loop_run(self->main_loop);
    g_clear_pointer(&self->main_loop, g_main_loop_unref);

    g_atomic_int_set(&self->event_thread_run, FALSE);
    if (self->event_thread) {
        libusb_interrupt_event_handler(NULL);
        g_thread_join(self->event_thread);
        self->event_thread = NULL;
    }

end:
    g_clear_pointer(&self->usbredirhost, usbredirhost_close);
    g_clear_object(&self->connection);
    g_free(self);
err_init:
    libusb_exit(NULL);

    if (err != NULL) {
        g_error_free(err);
        return 1;
    }

    return 0;
}
*/
import "C"

import (
	"context"
	"errors"
	"time"
	"unsafe"
)

func Run(ctx context.Context, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}

	cConfig := C.usbredir_config{
		device: C.device{
			vendor:        C.int(config.Vendor),
			product:       C.int(config.Product),
			bus:           C.int(config.Bus),
			device_number: C.int(config.DeviceNum),
		},
		net_settings: C.net_settings{
			addr:      C.CString(config.Address),
			port:      C.int(config.Port),
			keepalive: C.bool(config.KeepAlive),
		},
		verbosity: C.int(config.Verbosity),
	}
	defer C.free(unsafe.Pointer(cConfig.net_settings.addr))

	done := make(chan error, 1)
	go func() {
		//nolint:gocritic // C.usbredir_run returns 0 on success
		if C.usbredir_run(&cConfig) == 0 {
			done <- nil
		} else {
			done <- errors.New("failed to run usbredir")
		}
	}()

	const timeout = 10 * time.Second

	select {
	case <-ctx.Done():
		select {
		case err := <-done:
			return err
		case <-time.After(timeout):
			return errors.New("usbredir timeout")
		}
	case err := <-done:
		return err
	}
}
