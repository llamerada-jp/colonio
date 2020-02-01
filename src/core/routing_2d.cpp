/*
 * Copyright 2017-2020 Yuji Ito <llamerada.jp@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#include "routing_2d.hpp"

#include <cassert>

#include "context.hpp"
#include "convert.hpp"
#include "coord_system.hpp"
#include "utils.hpp"

namespace colonio {

Routing2D::Routing2D(Context& context_, RoutingAlgorithm2DDelegate& delegate_) :
    RoutingAlgorithm("2D"),
    context(context_),
    delegate(delegate_) {
}

const std::set<NodeID>& Routing2D::get_required_nodes() {
  return required_nodes;
}

void Routing2D::on_change_my_position(const Coordinate& position) {
  update_required_nodes();
}

bool Routing2D::on_change_online_links(const std::set<NodeID>& nids) {
  bool is_changed = false;

  for (auto& nid : nids) {
    auto it = connected_nodes.find(nid);
    if (it == connected_nodes.end()) {
      connected_nodes.insert(std::make_pair(nid, ConnectedNode()));
      is_changed = true;
    }
  }

  auto it = connected_nodes.begin();
  while (it != connected_nodes.end()) {
    const NodeID& nid = it->first;
    if (nids.find(nid) == nids.end()) {
      it         = connected_nodes.erase(it);
      is_changed = true;

    } else {
      it++;
    }
  }

  return is_changed;
}

void Routing2D::on_recv_packet(const NodeID& nid, const Packet& packet) {
  // Ignore
}

bool Routing2D::on_recv_routing_info(const Packet& packet, const RoutingProtocol::RoutingInfo& routing_info) {
  bool is_changed = false;
  // Ignore routing packet when source node has disconnected.
  // assert(connected_nodes.find(packet.src_nid) != connected_nodes.end());
  if (connected_nodes.find(packet.src_nid) == connected_nodes.end()) {
    printf("disconnected:%s\n", packet.src_nid.to_str().c_str());
    return false;
  }

  ConnectedNode& cn = connected_nodes.at(packet.src_nid);
  if (routing_info.has_r2d_position()) {
    Coordinate position = Coordinate::from_pb(routing_info.r2d_position());
    if (!cn.position.is_enable() || cn.position != position) {
      cn.position = position;
      is_changed  = true;
    }
  }
  cn.nexts.clear();

  const NodeID& local_nid = context.local_nid;

  for (auto& it : routing_info.nodes()) {
    NodeID nid = NodeID::from_str(it.first);
    if (local_nid != nid && it.second.has_r2d_position()) {
      Coordinate position = Coordinate::from_pb(it.second.r2d_position());
      cn.nexts.insert(std::make_pair(nid, position));
    }
  }

  return is_changed;
}

void Routing2D::send_routing_info(RoutingProtocol::RoutingInfo* param) {
  if (context.has_my_position()) {
    context.get_my_position().to_pb(param->mutable_r2d_position());

    for (auto& it_cn : connected_nodes) {
      if (it_cn.second.position.is_enable()) {
        std::string nid_str = it_cn.first.to_str();
        it_cn.second.position.to_pb((*param->mutable_nodes())[nid_str].mutable_r2d_position());
      }
    }
  }
}

bool Routing2D::update_routing_info(
    const std::set<NodeID>& online_links, bool has_update_ol,
    const std::map<NodeID, std::tuple<std::unique_ptr<const Packet>, RoutingProtocol::RoutingInfo>>& routing_infos) {
  bool is_changed = false;
  if (has_update_ol) {
    is_changed = is_changed || on_change_online_links(online_links);
  }
  for (auto& it : routing_infos) {
    is_changed = is_changed || on_recv_routing_info(*std::get<0>(it.second), std::get<1>(it.second));
  }

  if (is_changed) {
    update_required_nodes();
  }

  // Ignore
#ifndef NDEBUG
  picojson::object o;
  picojson::object nodes;
  picojson::array links;
  std::list<std::pair<const NodeID&, const NodeID&>> link_tmp;

  for (auto& it : connected_nodes) {
    if (it.second.position.is_enable()) {
      nodes.insert(std::make_pair(it.first.to_str(), Convert::coordinate2json(it.second.position)));
      const NodeID& n1 = NodeID::THIS;
      const NodeID& n2 = it.first == context.local_nid ? NodeID::THIS : it.first;
      link_tmp.push_back(
          n1 < n2 ? std::make_pair(std::ref(n1), std::ref(n2)) : std::make_pair(std::ref(n2), std::ref(n1)));
    }
  }

  for (auto& it1 : connected_nodes) {
    for (auto& it2 : it1.second.nexts) {
      if (it1.second.position.is_enable() && it2.second.is_enable()) {
        nodes.insert(std::make_pair(it2.first.to_str(), Convert::coordinate2json(it2.second)));
        const NodeID& n1 = it1.first == context.local_nid ? NodeID::THIS : it1.first;
        const NodeID& n2 = it2.first == context.local_nid ? NodeID::THIS : it2.first;
        link_tmp.push_back(
            n1 < n2 ? std::make_pair(std::ref(n1), std::ref(n2)) : std::make_pair(std::ref(n2), std::ref(n1)));
      }
    }
  }

  link_tmp.sort();
  link_tmp.unique();

  for (auto& it : link_tmp) {
    picojson::array one_pair;
    one_pair.push_back(it.first.to_json());
    one_pair.push_back(it.second.to_json());
    links.push_back(picojson::value(one_pair));
  }

  o.insert(std::make_pair("nodes", picojson::value(nodes)));
  o.insert(std::make_pair("links", picojson::value(links)));
  context.debug_event(DebugEvent::KNOWN2D, picojson::value(o));
#endif

  return is_changed;
}

const NodeID& Routing2D::get_relay_nid(const Coordinate& position) {
  const NodeID* near_nid = &NodeID::THIS;
  double min_distance    = std::numeric_limits<double>::max();

  for (auto& node : connected_nodes) {
    double distance = context.coord_system->get_distance(context.get_my_position(), node.second.position);
    if (distance < min_distance) {
      min_distance = distance;
      near_nid     = &node.first;
    }
  }

  return *near_nid;
}

Routing2D::NodePoint::NodePoint() : nid(NodeID::NONE) {
}

Routing2D::NodePoint::NodePoint(const NodeID& nid_, const Coordinate& position_, const Coordinate& sposition) :
    nid(nid_),
    position(position_),
    shifted_position(sposition) {
}

Routing2D::NodePoint& Routing2D::NodePoint::operator=(const NodePoint& r) {
  if (&r != this) {
    nid              = r.nid;
    position         = r.position;
    shifted_position = r.shifted_position;
  }

  return *this;
}

Routing2D::Triangle::Triangle(const NodePoint& p1, const NodePoint& p2, const NodePoint& p3) {
  std::map<Coordinate, const NodePoint*> work;
  work.insert(std::make_pair(p1.shifted_position, &p1));
  work.insert(std::make_pair(p2.shifted_position, &p2));
  work.insert(std::make_pair(p3.shifted_position, &p3));
  int i = 0;
  for (auto& it : work) {
    p[i] = *(it.second);
    i++;
  }
}

bool Routing2D::Triangle::operator<(const Triangle& t) const {
  if (p[0].shifted_position != t.p[0].shifted_position) {
    return p[0].shifted_position < t.p[0].shifted_position;

  } else if (p[1].shifted_position != t.p[1].shifted_position) {
    return p[1].shifted_position < t.p[1].shifted_position;

  } else {
    return p[2].shifted_position < t.p[2].shifted_position;
  }
}

bool Routing2D::Triangle::check_contain_node(const NodeID& nid) const {
  for (int i = 0; i < 3; i++) {
    if (p[i].nid == nid) {
      return true;
    }
  }
  return false;
}

bool Routing2D::Triangle::check_equal(const Triangle& t) const {
  for (int i = 0; i < 3; i++) {
    if (p[i].nid != t.p[i].nid || p[i].shifted_position != t.p[i].shifted_position) {
      return false;
    }
  }
  return true;
}

Routing2D::Circle Routing2D::Triangle::get_circumscribed_circle(Context& context) {
  double x1 = p[0].shifted_position.x;
  double y1 = p[0].shifted_position.y;
  double x2 = p[1].shifted_position.x;
  double y2 = p[1].shifted_position.y;
  double x3 = p[2].shifted_position.x;
  double y3 = p[2].shifted_position.y;

  double c = 2.0 * ((x2 - x1) * (y3 - y1) - (y2 - y1) * (x3 - x1));
  Circle ret;
  ret.center = Coordinate(
      ((y3 - y1) * (x2 * x2 - x1 * x1 + y2 * y2 - y1 * y1) + (y1 - y2) * (x3 * x3 - x1 * x1 + y3 * y3 - y1 * y1)) / c,
      ((x1 - x3) * (x2 * x2 - x1 * x1 + y2 * y2 - y1 * y1) + (x2 - x1) * (x3 * x3 - x1 * x1 + y3 * y3 - y1 * y1)) / c);
  ret.r = context.coord_system->get_distance(ret.center, p[0].shifted_position);

  return ret;
}

void Routing2D::delaunay_add_tmp_triangle(std::map<Triangle, bool>& tmp_triangles, const Triangle& triangle) {
  bool is_unique = true;
  for (auto& tmp_triangle : tmp_triangles) {
    if (triangle.check_equal(tmp_triangle.first)) {
      tmp_triangle.second = false;
      is_unique           = false;
    }
  }
  if (is_unique) {
    tmp_triangles.insert(std::make_pair(triangle, true));
  }
}

void Routing2D::delaunay_check_duplicate_point(std::map<NodeID, NodePoint>& nodes) {
  std::map<Coordinate, NodePoint*> checked;

  for (auto& node : nodes) {
    if (checked.find(node.second.shifted_position) == checked.end()) {
      checked.insert(std::make_pair(node.second.shifted_position, &node.second));

    } else {
      delaunay_shift_duplicate_point(*checked.at(node.second.shifted_position));
      delaunay_shift_duplicate_point(node.second);
    }
  }
}

Routing2D::Triangle Routing2D::delaunay_get_huge_triangle() {
  Coordinate center(
      (context.coord_system->MAX_X - context.coord_system->MIN_X) / 2,
      (context.coord_system->MAX_Y - context.coord_system->MIN_Y) / 2);
  double r =
      context.coord_system->get_distance(center, Coordinate(context.coord_system->MIN_X, context.coord_system->MIN_Y));
  Coordinate p1(center.x - sqrt(3) * r, center.y - r);
  Coordinate p2(center.x + sqrt(3) * r, center.y - r);
  Coordinate p3(center.x, center.y + 2 * r);

  return Triangle(NodePoint(NodeID::NONE, p1, p1), NodePoint(NodeID::NONE, p2, p2), NodePoint(NodeID::NONE, p3, p3));
}

/**
 * Make NodePoint list for delauney triangulation.
 * List including point of this node, and 1st neighborhood, and 2nd neighborhood without the same node-id.
 * List may contain duplicate coordinate NodePoint.
 */
void Routing2D::delaunay_make_nodes(std::map<NodeID, NodePoint>& nodes) {
  const NodeID& local_nid = context.local_nid;
  Coordinate base         = context.get_my_position();

  nodes.insert(std::make_pair(NodeID::THIS, NodePoint(NodeID::THIS, base, Coordinate(0.0, 0.0))));

  for (auto& it_cn : connected_nodes) {
    if (nodes.find(it_cn.first) == nodes.end() && it_cn.second.position.is_enable()) {
      assert(it_cn.first != local_nid);
      nodes.insert(std::make_pair(
          it_cn.first, NodePoint(
                           it_cn.first, it_cn.second.position,
                           context.coord_system->shift_for_routing_2d(base, it_cn.second.position))));

    } else if (it_cn.second.position.is_enable()) {
      auto it_nodes                     = nodes.find(it_cn.first);
      it_nodes->second.shifted_position = context.coord_system->shift_for_routing_2d(base, it_cn.second.position);
    }

    for (auto& it_next : it_cn.second.nexts) {
      if (nodes.find(it_next.first) == nodes.end() && it_next.second.is_enable()) {
        assert(it_next.first != local_nid);
        nodes.insert(std::make_pair(
            it_next.first,
            NodePoint(
                it_next.first, it_next.second, context.coord_system->shift_for_routing_2d(base, it_next.second))));
      }
    }
  }
}

void Routing2D::delaunay_shift_duplicate_point(NodePoint& np) {
  uint64_t id0;
  uint64_t id1;
  np.nid.get_raw(&id0, &id1);
  double shift_x = (context.coord_system->PRECISION * id0) / static_cast<double>(UINT64_MAX);
  double shift_y = (context.coord_system->PRECISION * id1) / static_cast<double>(UINT64_MAX);
  np.shifted_position.x += shift_x;
  np.shifted_position.y += shift_y;
}

void Routing2D::update_required_nodes() {
  // Collect node points.
  std::map<NodeID, NodePoint> nodes;
  delaunay_make_nodes(nodes);
  delaunay_check_duplicate_point(nodes);

  // Make delaunay triangles.
  std::list<Triangle> triangles;
  triangles.push_back(delaunay_get_huge_triangle());

  for (auto& it_node : nodes) {
    std::map<Triangle, bool> tmp_triangles;
    const NodeID& nid           = it_node.first;
    const NodePoint& node_point = it_node.second;

    auto it_triangle = triangles.begin();
    while (it_triangle != triangles.end()) {
      Circle c = it_triangle->get_circumscribed_circle(context);
      if (context.coord_system->get_distance(c.center, node_point.shifted_position) <= c.r) {
        delaunay_add_tmp_triangle(tmp_triangles, Triangle(node_point, it_triangle->p[0], it_triangle->p[1]));
        delaunay_add_tmp_triangle(tmp_triangles, Triangle(node_point, it_triangle->p[1], it_triangle->p[2]));
        delaunay_add_tmp_triangle(tmp_triangles, Triangle(node_point, it_triangle->p[2], it_triangle->p[0]));
        it_triangle = triangles.erase(it_triangle);

      } else {
        it_triangle++;
      }
    }

    for (auto& tmp_triangle : tmp_triangles) {
      if (tmp_triangle.second) {
        triangles.push_back(tmp_triangle.first);
      }
    }
  }

  // Update require_nodes.
  required_nodes.clear();
  for (auto& triangle : triangles) {
    if (triangle.check_contain_node(NodeID::THIS)) {
      for (auto& np : triangle.p) {
        if (np.nid != NodeID::THIS && np.nid != NodeID::NONE) {
          required_nodes.insert(np.nid);
        }
      }
    }
  }

  // Check change for nearby node info.
  bool is_changed_nearby         = false;
  bool is_changed_nearby_positon = false;
  for (auto& nid : required_nodes) {
    auto it = nearby_nodes.find(nid);
    if (it == nearby_nodes.end()) {
      nearby_nodes.insert(std::make_pair(nid, nodes.at(nid).position));
      is_changed_nearby         = true;
      is_changed_nearby_positon = true;
    }
  }
  auto it_nn = nearby_nodes.begin();
  while (it_nn != nearby_nodes.end()) {
    if (required_nodes.find(it_nn->first) == required_nodes.end()) {
      it_nn                     = nearby_nodes.erase(it_nn);
      is_changed_nearby         = true;
      is_changed_nearby_positon = true;

    } else {
      const Coordinate& nearby_position = nodes.at(it_nn->first).position;
      if (it_nn->second != nearby_position) {
        it_nn->second             = nearby_position;
        is_changed_nearby_positon = true;
      }

      it_nn++;
    }
  }
  if (is_changed_nearby) {
    delegate.algorithm_2d_on_change_nearby(*this, required_nodes);
  }
  if (is_changed_nearby_positon) {
    delegate.algorithm_2d_on_change_nearby_position(*this, nearby_nodes);
  }

#ifndef NDEBUG
  {
    picojson::object o;
    for (auto& nid : required_nodes) {
      o.insert(std::make_pair(nid.to_str(), Convert::coordinate2json(nodes.at(nid).position)));
    }
    context.debug_event(DebugEvent::REQUIRED2D, picojson::value(o));
  }
#endif
}
}  // namespace colonio
