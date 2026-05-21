use crate::prelude::*;
use crate::globals::GLOBAL_REQ_CHAN;
use crate::bridge::zmq_packet_dispatcher::dispatch_zmq_packet;

pub fn run() {
    let receiver_handler = thread::spawn(|| {
        let context = zmq::Context::new();
        let responder = Arc::new(Mutex::new(context.socket(zmq::REP).unwrap()));
        {
            let res_lock = responder.lock().unwrap();
            assert!(res_lock.bind("tcp://*:5556").is_ok());
        }
        let mut msg = zmq::Message::new();
        loop {
            let response_payload;
            {
                let res_lock = responder.lock().unwrap();
                res_lock.recv(&mut msg, 0).unwrap();
            }
            let data = msg.as_str().unwrap_or("{}");
            println!("recevied {data}");
            let packet: JsonValue = serde_json::from_str(data).unwrap_or_else(|_| json!({}));
            response_payload = dispatch_zmq_packet(&packet);

            {
                let res_lock = responder.lock().unwrap();
                res_lock.send(response_payload.as_bytes(), 0).unwrap();
            }
        }
    });

    let chan = GLOBAL_REQ_CHAN.clone();
    let sender_handler = thread::spawn(move || {
        println!("Connecting to host platform server...\n");
        let context = zmq::Context::new();
        let requester = context.socket(zmq::REQ).unwrap();
        assert!(requester.connect("tcp://localhost:5555").is_ok());
        let mut msg = zmq::Message::new();
        loop {
            let packet = chan.pop();
            requester.send(&packet, 0).unwrap();
            requester.recv(&mut msg, 0).unwrap();
        }
    });

    // On-chain execution pipeline is removed. Appengine now only serves runtime VM execution.
    receiver_handler.join().unwrap();
    sender_handler.join().unwrap();
}
